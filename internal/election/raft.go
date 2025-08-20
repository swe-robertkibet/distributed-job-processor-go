package election

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"distributed-job-processor/internal/logger"
	"distributed-job-processor/internal/storage"

	"github.com/sirupsen/logrus"
)

type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
)

func (s RaftState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

type LogEntry struct {
	Term    int64                  `json:"term" bson:"term"`
	Index   int64                  `json:"index" bson:"index"`
	Data    map[string]interface{} `json:"data" bson:"data"`
	Timestamp time.Time            `json:"timestamp" bson:"timestamp"`
}

type RaftNode struct {
	ID       string    `json:"id" bson:"_id"`
	Address  string    `json:"address" bson:"address"`
	LastSeen time.Time `json:"last_seen" bson:"last_seen"`
	Term     int64     `json:"term" bson:"term"`
}

type VoteRequest struct {
	Term         int64  `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex int64  `json:"last_log_index"`
	LastLogTerm  int64  `json:"last_log_term"`
}

type VoteResponse struct {
	Term        int64 `json:"term"`
	VoteGranted bool  `json:"vote_granted"`
}

type AppendEntriesRequest struct {
	Term         int64       `json:"term"`
	LeaderID     string      `json:"leader_id"`
	PrevLogIndex int64       `json:"prev_log_index"`
	PrevLogTerm  int64       `json:"prev_log_term"`
	Entries      []LogEntry  `json:"entries"`
	LeaderCommit int64       `json:"leader_commit"`
}

type AppendEntriesResponse struct {
	Term    int64 `json:"term"`
	Success bool  `json:"success"`
}

type RaftElection struct {
	nodeID    string
	state     RaftState
	storage   *storage.MongoStorage
	
	currentTerm int64
	votedFor    string
	log         []LogEntry
	
	commitIndex int64
	lastApplied int64
	
	nextIndex  map[string]int64
	matchIndex map[string]int64
	
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration
	
	lastHeartbeat time.Time
	votes         map[string]bool
	
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	callbacks []LeadershipCallback
	
	electionTimer *time.Timer
	heartbeatTimer *time.Timer
}

func NewRaftElection(nodeID string, storage *storage.MongoStorage, electionTimeout, heartbeatTimeout time.Duration) *RaftElection {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &RaftElection{
		nodeID:           nodeID,
		state:            Follower,
		storage:          storage,
		currentTerm:      0,
		votedFor:         "",
		log:              make([]LogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		nextIndex:        make(map[string]int64),
		matchIndex:       make(map[string]int64),
		electionTimeout:  electionTimeout,
		heartbeatTimeout: heartbeatTimeout,
		lastHeartbeat:    time.Now(),
		votes:            make(map[string]bool),
		ctx:              ctx,
		cancel:           cancel,
	}
}

func (r *RaftElection) Start() {
	logger.WithField("node_id", r.nodeID).Info("Starting Raft election algorithm")
	
	r.mu.Lock()
	r.state = Follower
	r.resetElectionTimer()
	r.mu.Unlock()
	
	go r.run()
}

func (r *RaftElection) Stop() {
	logger.Info("Stopping Raft election")
	
	// First update state and stop timers before cancelling context
	// This prevents deadlocks where context cancellation happens before state updates
	r.mu.Lock()
	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
	if r.heartbeatTimer != nil {
		r.heartbeatTimer.Stop()
	}
	
	// Update state to ensure no new operations start
	if r.state == Leader {
		r.state = Follower
		r.updateNodeInfo(false)
	}
	r.mu.Unlock()
	
	// Cancel context after state is properly updated
	r.cancel()
}

func (r *RaftElection) IsLeader() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state == Leader
}

func (r *RaftElection) GetLeader() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if r.state == Leader {
		return r.nodeID
	}
	
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		return ""
	}
	
	for _, node := range nodes {
		if node.IsLeader {
			return node.ID
		}
	}
	
	return ""
}

func (r *RaftElection) AddCallback(callback LeadershipCallback) {
	r.callbacks = append(r.callbacks, callback)
}

func (r *RaftElection) run() {
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			r.mu.RLock()
			state := r.state
			r.mu.RUnlock()
			
			switch state {
			case Follower:
				r.runFollower()
			case Candidate:
				r.runCandidate()
			case Leader:
				r.runLeader()
			}
		}
	}
}

func (r *RaftElection) runFollower() {
	select {
	case <-r.ctx.Done():
		return
	case <-r.electionTimer.C:
		logger.WithField("node_id", r.nodeID).Info("Election timeout, becoming candidate")
		r.becomeCandidate()
	}
}

func (r *RaftElection) runCandidate() {
	r.mu.Lock()
	r.currentTerm++
	r.votedFor = r.nodeID
	r.votes = make(map[string]bool)
	r.votes[r.nodeID] = true
	r.resetElectionTimer()
	term := r.currentTerm
	r.mu.Unlock()
	
	logger.WithFields(logrus.Fields{
		"node_id": r.nodeID,
		"term":    term,
	}).Info("Starting election as candidate")
	
	go r.requestVotes()
	
	// Wait for either election timeout or enough time to collect votes
	maxWaitTime := r.electionTimeout / 3  // Give more time to collect votes
	ticker := time.NewTicker(maxWaitTime / 10)  // Check periodically
	defer ticker.Stop()
	
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.electionTimer.C:
			logger.WithField("node_id", r.nodeID).Info("Election timeout, restarting election")
			r.becomeCandidate()
			return
		case <-ticker.C:
			r.mu.RLock()
			voteCount := len(r.votes)
			currentState := r.state
			r.mu.RUnlock()
			
			// Check if we're still a candidate
			if currentState != Candidate {
				return
			}
			
			if r.hasMajority(voteCount) {
				r.becomeLeader()
				return
			}
		case <-time.After(maxWaitTime):
			// Final check after maximum wait time
			r.mu.RLock()
			voteCount := len(r.votes)
			r.mu.RUnlock()
			
			if r.hasMajority(voteCount) {
				r.becomeLeader()
			} else {
				logger.WithFields(logrus.Fields{
					"node_id": r.nodeID,
					"votes": voteCount,
				}).Info("Election failed - insufficient votes")
				r.becomeFollower(r.currentTerm)
			}
			return
		}
	}
}

func (r *RaftElection) runLeader() {
	logger.WithField("node_id", r.nodeID).Info("Running as leader")
	
	r.mu.Lock()
	if r.heartbeatTimer != nil {
		r.heartbeatTimer.Stop()
	}
	r.heartbeatTimer = time.NewTimer(r.heartbeatTimeout)
	r.mu.Unlock()
	
	select {
	case <-r.ctx.Done():
		return
	case <-r.heartbeatTimer.C:
		go r.sendHeartbeats()
		r.mu.Lock()
		r.heartbeatTimer.Reset(r.heartbeatTimeout)
		r.mu.Unlock()
	}
}

func (r *RaftElection) becomeFollower(term int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if term > r.currentTerm {
		r.currentTerm = term
		r.votedFor = ""
	}
	
	oldState := r.state
	r.state = Follower
	r.lastHeartbeat = time.Now()
	r.resetElectionTimer()
	
	if oldState != Follower {
		logger.WithFields(logrus.Fields{
			"node_id": r.nodeID,
			"term":    r.currentTerm,
		}).Info("Became follower")
		
		r.updateNodeInfo(false)
		r.notifyCallbacks()
	}
}

func (r *RaftElection) becomeCandidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Add small random delay before becoming candidate to prevent synchronized candidacy
	delay := time.Duration(rand.Intn(50)) * time.Millisecond
	r.mu.Unlock()
	time.Sleep(delay)
	r.mu.Lock()
	
	// Check if we should still become candidate after delay
	if r.state != Follower {
		return
	}
	
	r.state = Candidate
	logger.WithField("node_id", r.nodeID).Info("Became candidate")
}

func (r *RaftElection) becomeLeader() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Validate we're still a candidate and have the right term
	if r.state != Candidate {
		logger.WithField("node_id", r.nodeID).Info("Cannot become leader - not a candidate")
		return
	}
	
	// Double-check we still have majority with current votes
	voteCount := len(r.votes)
	if !r.hasMajorityUnsafe(voteCount) {
		logger.WithFields(logrus.Fields{
			"node_id": r.nodeID,
			"votes": voteCount,
		}).Info("Cannot become leader - lost majority")
		r.becomeFollowerUnsafe(r.currentTerm)
		return
	}
	
	// Check if another leader already exists in this term by trying to register as leader atomically
	if !r.tryClaimLeadership() {
		logger.WithField("node_id", r.nodeID).Info("Cannot become leader - another leader exists in current term")
		r.becomeFollowerUnsafe(r.currentTerm)
		return
	}
	
	oldState := r.state
	r.state = Leader
	
	if oldState != Leader {
		logger.WithFields(logrus.Fields{
			"node_id": r.nodeID,
			"term":    r.currentTerm,
		}).Info("Became leader")
		
		r.initializeLeaderState()
		r.updateNodeInfo(true)
		r.notifyCallbacks()
	}
}

func (r *RaftElection) initializeLeaderState() {
	r.nextIndex = make(map[string]int64)
	r.matchIndex = make(map[string]int64)
	
	lastLogIndex := int64(len(r.log))
	
	nodes, err := r.storage.GetNodes(r.ctx)
	if err == nil {
		for _, node := range nodes {
			if node.ID != r.nodeID {
				r.nextIndex[node.ID] = lastLogIndex + 1
				r.matchIndex[node.ID] = 0
			}
		}
	}
}

func (r *RaftElection) requestVotes() {
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to get nodes for vote request")
		return
	}
	
	r.mu.RLock()
	request := VoteRequest{
		Term:         r.currentTerm,
		CandidateID:  r.nodeID,
		LastLogIndex: int64(len(r.log)),
		LastLogTerm:  r.getLastLogTerm(),
	}
	r.mu.RUnlock()
	
	for _, node := range nodes {
		if node.ID != r.nodeID {
			go r.sendVoteRequest(node.ID, request)
		}
	}
}

func (r *RaftElection) sendVoteRequest(nodeID string, request VoteRequest) {
	logger.WithFields(logrus.Fields{
		"node_id":    r.nodeID,
		"target":     nodeID,
		"term":       request.Term,
	}).Debug("Sending vote request")
	
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
	
	response := VoteResponse{
		Term:        request.Term,
		VoteGranted: rand.Float32() > 0.3,
	}
	
	r.handleVoteResponse(nodeID, response)
}

func (r *RaftElection) handleVoteResponse(nodeID string, response VoteResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if r.state != Candidate || response.Term != r.currentTerm {
		return
	}
	
	if response.Term > r.currentTerm {
		r.currentTerm = response.Term
		r.votedFor = ""
		r.state = Follower
		r.resetElectionTimer()
		return
	}
	
	if response.VoteGranted {
		r.votes[nodeID] = true
		logger.WithFields(logrus.Fields{
			"node_id":    r.nodeID,
			"voter":      nodeID,
			"vote_count": len(r.votes),
		}).Debug("Received vote")
	}
}

func (r *RaftElection) sendHeartbeats() {
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to get nodes for heartbeat")
		return
	}
	
	r.mu.RLock()
	request := AppendEntriesRequest{
		Term:         r.currentTerm,
		LeaderID:     r.nodeID,
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{},
		LeaderCommit: r.commitIndex,
	}
	r.mu.RUnlock()
	
	for _, node := range nodes {
		if node.ID != r.nodeID {
			go r.sendAppendEntries(node.ID, request)
		}
	}
	
	r.updateNodeInfo(true)
}

func (r *RaftElection) sendAppendEntries(nodeID string, request AppendEntriesRequest) {
	logger.WithFields(logrus.Fields{
		"node_id": r.nodeID,
		"target":  nodeID,
		"term":    request.Term,
	}).Debug("Sending heartbeat")
	
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
}

func (r *RaftElection) hasMajority(voteCount int) bool {
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		return false
	}
	
	totalNodes := len(nodes)
	return voteCount > totalNodes/2
}

func (r *RaftElection) hasMajorityUnsafe(voteCount int) bool {
	// Unsafe version for use when already holding lock
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		return false
	}
	
	totalNodes := len(nodes)
	return voteCount > totalNodes/2
}

func (r *RaftElection) hasLeaderInCurrentTerm() bool {
	// Check if another node is already leader in current term
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		return false
	}
	
	for _, node := range nodes {
		if node.ID != r.nodeID && node.IsLeader {
			// Check if this leader is in a valid state
			if time.Since(node.LastSeen) < r.heartbeatTimeout*3 {
				return true
			}
		}
	}
	return false
}

func (r *RaftElection) tryClaimLeadership() bool {
	// First clear any stale leaders
	r.clearStaleLeaders()
	
	// Try to register as leader - this should be atomic
	nodeInfo := &storage.NodeInfo{
		ID:          r.nodeID,
		Address:     "localhost:8080",
		IsLeader:    true,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	// Check if we can successfully register as leader
	if err := r.storage.RegisterNode(r.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to register as leader")
		return false
	}
	
	// Double-check that we're the only leader
	time.Sleep(10 * time.Millisecond) // Small delay to ensure other nodes have updated
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		return false
	}
	
	leaderCount := 0
	for _, node := range nodes {
		if node.IsLeader && time.Since(node.LastSeen) < r.heartbeatTimeout {
			leaderCount++
			if node.ID != r.nodeID {
				// Another node is also leader - conflict detected
				logger.WithFields(logrus.Fields{
					"node_id": r.nodeID,
					"other_leader": node.ID,
				}).Warning("Leadership conflict detected - stepping down")
				
				// Step down immediately
				nodeInfo.IsLeader = false
				r.storage.RegisterNode(r.ctx, nodeInfo)
				return false
			}
		}
	}
	
	return leaderCount == 1
}

func (r *RaftElection) clearStaleLeaders() {
	nodes, err := r.storage.GetNodes(r.ctx)
	if err != nil {
		return
	}
	
	for _, node := range nodes {
		if node.IsLeader && time.Since(node.LastSeen) > r.heartbeatTimeout*3 {
			// Clear stale leader
			nodeInfo := &storage.NodeInfo{
				ID:          node.ID,
				Address:     node.Address,
				IsLeader:    false,
				WorkerCount: node.WorkerCount,
				Version:     node.Version,
			}
			r.storage.RegisterNode(r.ctx, nodeInfo)
		}
	}
}

func (r *RaftElection) becomeFollowerUnsafe(term int64) {
	// Unsafe version for use when already holding lock
	if term > r.currentTerm {
		r.currentTerm = term
		r.votedFor = ""
	}
	
	if r.state != Follower {
		r.state = Follower
		r.votes = make(map[string]bool)
		
		if r.electionTimer != nil {
			r.electionTimer.Stop()
		}
		r.resetElectionTimer()
		
		logger.WithFields(logrus.Fields{
			"node_id": r.nodeID,
			"term":    r.currentTerm,
		}).Info("Became follower")
		
		r.updateNodeInfo(false)
		r.notifyCallbacks()
	}
}

func (r *RaftElection) getLastLogTerm() int64 {
	if len(r.log) == 0 {
		return 0
	}
	return r.log[len(r.log)-1].Term
}

func (r *RaftElection) resetElectionTimer() {
	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
	
	// Add significant randomization to prevent synchronized elections
	// Range: electionTimeout to 2*electionTimeout with additional jitter
	baseTimeout := r.electionTimeout
	randomFactor := time.Duration(rand.Intn(int(baseTimeout/time.Millisecond))) * time.Millisecond
	jitter := time.Duration(rand.Intn(200)) * time.Millisecond // 0-200ms additional jitter
	
	timeout := baseTimeout + randomFactor + jitter
	
	logger.WithFields(logrus.Fields{
		"node_id": r.nodeID,
		"timeout": timeout,
		"base":    baseTimeout,
	}).Debug("Reset election timer with randomization")
	
	r.electionTimer = time.NewTimer(timeout)
}

func (r *RaftElection) updateNodeInfo(isLeader bool) {
	nodeInfo := &storage.NodeInfo{
		ID:          r.nodeID,
		Address:     "localhost:8080",
		IsLeader:    isLeader,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	if err := r.storage.RegisterNode(r.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to update node info")
	}
}

func (r *RaftElection) notifyCallbacks() {
	// Get leader ID without acquiring lock again (we're already holding it)
	var leaderID string
	isLeader := r.state == Leader
	
	if isLeader {
		leaderID = r.nodeID
	} else {
		// Check storage for current leader without acquiring lock
		nodes, err := r.storage.GetNodes(r.ctx)
		if err == nil {
			for _, node := range nodes {
				if node.IsLeader {
					leaderID = node.ID
					break
				}
			}
		}
	}
	
	for _, callback := range r.callbacks {
		go callback(isLeader, leaderID)
	}
}