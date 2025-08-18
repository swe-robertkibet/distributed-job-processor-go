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
	r.cancel()
	
	r.mu.Lock()
	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
	if r.heartbeatTimer != nil {
		r.heartbeatTimer.Stop()
	}
	r.mu.Unlock()
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
	
	select {
	case <-r.ctx.Done():
		return
	case <-r.electionTimer.C:
		logger.WithField("node_id", r.nodeID).Info("Election timeout, restarting election")
		r.becomeCandidate()
	case <-time.After(r.electionTimeout / 2):
		r.mu.RLock()
		voteCount := len(r.votes)
		r.mu.RUnlock()
		
		if r.hasMajority(voteCount) {
			r.becomeLeader()
		} else {
			r.becomeFollower(r.currentTerm)
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
	
	r.state = Candidate
	logger.WithField("node_id", r.nodeID).Info("Became candidate")
}

func (r *RaftElection) becomeLeader() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
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
	
	timeout := r.electionTimeout + time.Duration(rand.Intn(int(r.electionTimeout/time.Millisecond)))*time.Millisecond
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
	leaderID := r.GetLeader()
	isLeader := r.state == Leader
	
	for _, callback := range r.callbacks {
		go callback(isLeader, leaderID)
	}
}