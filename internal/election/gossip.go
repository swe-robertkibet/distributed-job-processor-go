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

type GossipNodeState int

const (
	Alive GossipNodeState = iota
	Suspected
	Failed
)

func (s GossipNodeState) String() string {
	switch s {
	case Alive:
		return "Alive"
	case Suspected:
		return "Suspected"
	case Failed:
		return "Failed"
	default:
		return "Unknown"
	}
}

type GossipMember struct {
	ID          string           `json:"id"`
	Address     string           `json:"address"`
	State       GossipNodeState  `json:"state"`
	Incarnation int64            `json:"incarnation"`
	LastSeen    time.Time        `json:"last_seen"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type GossipMessage struct {
	Type        string                 `json:"type"`
	SenderID    string                 `json:"sender_id"`
	Members     []GossipMember         `json:"members,omitempty"`
	Suspicion   *SuspicionInfo         `json:"suspicion,omitempty"`
	Leadership  *LeadershipInfo        `json:"leadership,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

type SuspicionInfo struct {
	TargetID    string `json:"target_id"`
	SuspectorID string `json:"suspector_id"`
	Incarnation int64  `json:"incarnation"`
}

type LeadershipInfo struct {
	LeaderID    string    `json:"leader_id"`
	Term        int64     `json:"term"`
	Priority    float64   `json:"priority"`
	Timestamp   time.Time `json:"timestamp"`
}

type GossipElection struct {
	nodeID       string
	address      string
	storage      *storage.MongoStorage
	
	members      map[string]*GossipMember
	incarnation  int64
	isLeader     bool
	currentLeader string
	leaderTerm   int64
	priority     float64
	
	gossipInterval    time.Duration
	suspicionTimeout  time.Duration
	failureTimeout    time.Duration
	
	lastGossip       time.Time
	suspicions       map[string]*SuspicionInfo
	
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	callbacks []LeadershipCallback
	
	gossipTimer *time.Timer
}

func NewGossipElection(nodeID string, storage *storage.MongoStorage, gossipInterval, suspicionTimeout time.Duration) *GossipElection {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &GossipElection{
		nodeID:           nodeID,
		address:          "localhost:8080",
		storage:          storage,
		members:          make(map[string]*GossipMember),
		incarnation:      time.Now().Unix(),
		isLeader:         false,
		currentLeader:    "",
		leaderTerm:       0,
		priority:         rand.Float64(),
		gossipInterval:   gossipInterval,
		suspicionTimeout: suspicionTimeout,
		failureTimeout:   suspicionTimeout * 2,
		lastGossip:       time.Now(),
		suspicions:       make(map[string]*SuspicionInfo),
		ctx:              ctx,
		cancel:           cancel,
	}
}

func (g *GossipElection) Start() {
	logger.WithField("node_id", g.nodeID).Info("Starting Gossip election algorithm")
	
	g.mu.Lock()
	g.members[g.nodeID] = &GossipMember{
		ID:          g.nodeID,
		Address:     g.address,
		State:       Alive,
		Incarnation: g.incarnation,
		LastSeen:    time.Now(),
		Metadata: map[string]interface{}{
			"priority": g.priority,
		},
	}
	g.resetGossipTimer()
	g.mu.Unlock()
	
	go g.run()
	go g.runMembershipMaintenance()
}

func (g *GossipElection) Stop() {
	logger.Info("Stopping Gossip election")
	g.cancel()
	
	g.mu.Lock()
	if g.gossipTimer != nil {
		g.gossipTimer.Stop()
	}
	g.mu.Unlock()
}

func (g *GossipElection) IsLeader() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.isLeader
}

func (g *GossipElection) GetLeader() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentLeader
}

func (g *GossipElection) AddCallback(callback LeadershipCallback) {
	g.callbacks = append(g.callbacks, callback)
}

func (g *GossipElection) run() {
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-g.gossipTimer.C:
			g.performGossipRound()
			g.resetGossipTimer()
		}
	}
}

func (g *GossipElection) runMembershipMaintenance() {
	ticker := time.NewTicker(g.suspicionTimeout / 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.updateMemberStates()
			g.electLeader()
		}
	}
}

func (g *GossipElection) performGossipRound() {
	g.mu.RLock()
	
	memberList := make([]GossipMember, 0, len(g.members))
	for _, member := range g.members {
		memberList = append(memberList, *member)
	}
	
	message := GossipMessage{
		Type:      "membership",
		SenderID:  g.nodeID,
		Members:   memberList,
		Timestamp: time.Now(),
	}
	
	if g.isLeader {
		message.Leadership = &LeadershipInfo{
			LeaderID:  g.nodeID,
			Term:      g.leaderTerm,
			Priority:  g.priority,
			Timestamp: time.Now(),
		}
	}
	
	g.mu.RUnlock()
	
	g.discoverNodes()
	targets := g.selectGossipTargets(3)
	
	for _, target := range targets {
		go g.sendGossipMessage(target, message)
	}
	
	logger.WithFields(logrus.Fields{
		"node_id":     g.nodeID,
		"targets":     len(targets),
		"member_count": len(memberList),
	}).Debug("Performed gossip round")
}

func (g *GossipElection) discoverNodes() {
	nodes, err := g.storage.GetNodes(g.ctx)
	if err != nil {
		return
	}
	
	g.mu.Lock()
	defer g.mu.Unlock()
	
	for _, nodeInfo := range nodes {
		if nodeInfo.ID != g.nodeID {
			if _, exists := g.members[nodeInfo.ID]; !exists {
				g.members[nodeInfo.ID] = &GossipMember{
					ID:          nodeInfo.ID,
					Address:     nodeInfo.Address,
					State:       Alive,
					Incarnation: time.Now().Unix(),
					LastSeen:    nodeInfo.LastSeen,
					Metadata: map[string]interface{}{
						"priority": rand.Float64(),
					},
				}
				
				logger.WithField("node_id", nodeInfo.ID).Info("Discovered new node")
			} else {
				g.members[nodeInfo.ID].LastSeen = nodeInfo.LastSeen
			}
		}
	}
}

func (g *GossipElection) selectGossipTargets(count int) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var targets []string
	for id, member := range g.members {
		if id != g.nodeID && member.State == Alive {
			targets = append(targets, id)
		}
	}
	
	if len(targets) <= count {
		return targets
	}
	
	selected := make([]string, 0, count)
	indices := rand.Perm(len(targets))
	for i := 0; i < count && i < len(indices); i++ {
		selected = append(selected, targets[indices[i]])
	}
	
	return selected
}

func (g *GossipElection) sendGossipMessage(targetID string, message GossipMessage) {
	logger.WithFields(logrus.Fields{
		"node_id": g.nodeID,
		"target":  targetID,
		"type":    message.Type,
	}).Debug("Sending gossip message")
	
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
	
	g.simulateGossipResponse(targetID, message)
}

func (g *GossipElection) simulateGossipResponse(targetID string, message GossipMessage) {
	responseMembers := make([]GossipMember, 0)
	
	g.mu.RLock()
	for _, member := range g.members {
		if rand.Float32() > 0.7 {
			responseMembers = append(responseMembers, *member)
		}
	}
	g.mu.RUnlock()
	
	response := GossipMessage{
		Type:      "membership",
		SenderID:  targetID,
		Members:   responseMembers,
		Timestamp: time.Now(),
	}
	
	g.handleGossipMessage(response)
}

func (g *GossipElection) handleGossipMessage(message GossipMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	switch message.Type {
	case "membership":
		g.mergeMemberList(message.Members)
		
		if message.Leadership != nil {
			g.handleLeadershipInfo(message.Leadership)
		}
		
	case "suspicion":
		if message.Suspicion != nil {
			g.handleSuspicion(message.Suspicion)
		}
	}
}

func (g *GossipElection) mergeMemberList(members []GossipMember) {
	for _, member := range members {
		if member.ID == g.nodeID {
			continue
		}
		
		existing, exists := g.members[member.ID]
		if !exists {
			memberCopy := member
			g.members[member.ID] = &memberCopy
			
			logger.WithField("node_id", member.ID).Info("Added new member from gossip")
		} else {
			if member.Incarnation > existing.Incarnation {
				existing.Incarnation = member.Incarnation
				existing.State = member.State
				existing.LastSeen = member.LastSeen
				existing.Metadata = member.Metadata
			}
		}
	}
}

func (g *GossipElection) handleLeadershipInfo(leadership *LeadershipInfo) {
	if leadership.Term > g.leaderTerm {
		oldLeader := g.currentLeader
		g.currentLeader = leadership.LeaderID
		g.leaderTerm = leadership.Term
		g.isLeader = (leadership.LeaderID == g.nodeID)
		
		if oldLeader != g.currentLeader {
			logger.WithFields(logrus.Fields{
				"new_leader": g.currentLeader,
				"term":       g.leaderTerm,
			}).Info("Leadership changed")
			
			g.updateNodeInfo()
			g.notifyCallbacks()
		}
	}
}

func (g *GossipElection) handleSuspicion(suspicion *SuspicionInfo) {
	member, exists := g.members[suspicion.TargetID]
	if !exists {
		return
	}
	
	if suspicion.Incarnation >= member.Incarnation {
		if member.State == Alive {
			member.State = Suspected
			g.suspicions[suspicion.TargetID] = suspicion
			
			logger.WithFields(logrus.Fields{
				"node_id":   suspicion.TargetID,
				"suspector": suspicion.SuspectorID,
			}).Warn("Node suspected")
		}
	}
}

func (g *GossipElection) updateMemberStates() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	now := time.Now()
	
	for id, member := range g.members {
		if id == g.nodeID {
			continue
		}
		
		timeSinceLastSeen := now.Sub(member.LastSeen)
		
		switch member.State {
		case Alive:
			if timeSinceLastSeen > g.suspicionTimeout {
				member.State = Suspected
				g.suspicions[id] = &SuspicionInfo{
					TargetID:    id,
					SuspectorID: g.nodeID,
					Incarnation: member.Incarnation,
				}
				
				logger.WithField("node_id", id).Warn("Suspecting node")
				g.propagateSuspicion(id)
			}
			
		case Suspected:
			if timeSinceLastSeen > g.failureTimeout {
				member.State = Failed
				delete(g.suspicions, id)
				
				logger.WithField("node_id", id).Error("Node failed")
			}
		}
	}
	
	for id, member := range g.members {
		if member.State == Failed && time.Since(member.LastSeen) > g.failureTimeout*2 {
			delete(g.members, id)
			logger.WithField("node_id", id).Info("Removed failed node")
		}
	}
}

func (g *GossipElection) propagateSuspicion(targetID string) {
	suspicion := g.suspicions[targetID]
	if suspicion == nil {
		return
	}
	
	message := GossipMessage{
		Type:      "suspicion",
		SenderID:  g.nodeID,
		Suspicion: suspicion,
		Timestamp: time.Now(),
	}
	
	targets := g.selectGossipTargets(2)
	for _, target := range targets {
		go g.sendGossipMessage(target, message)
	}
}

func (g *GossipElection) electLeader() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	var candidates []string
	highestPriority := -1.0
	
	for id, member := range g.members {
		if member.State == Alive {
			if priority, ok := member.Metadata["priority"].(float64); ok {
				if priority > highestPriority {
					highestPriority = priority
					candidates = []string{id}
				} else if priority == highestPriority {
					candidates = append(candidates, id)
				}
			}
		}
	}
	
	if len(candidates) == 0 {
		return
	}
	
	var newLeader string
	if len(candidates) == 1 {
		newLeader = candidates[0]
	} else {
		for _, candidate := range candidates {
			if candidate < newLeader || newLeader == "" {
				newLeader = candidate
			}
		}
	}
	
	if newLeader != g.currentLeader {
		oldLeader := g.currentLeader
		g.currentLeader = newLeader
		g.leaderTerm++
		g.isLeader = (newLeader == g.nodeID)
		
		logger.WithFields(logrus.Fields{
			"old_leader": oldLeader,
			"new_leader": newLeader,
			"term":       g.leaderTerm,
			"priority":   highestPriority,
		}).Info("Leader elected")
		
		g.updateNodeInfo()
		g.notifyCallbacks()
		
		if g.isLeader {
			g.propagateLeadership()
		}
	}
}

func (g *GossipElection) propagateLeadership() {
	leadership := &LeadershipInfo{
		LeaderID:  g.nodeID,
		Term:      g.leaderTerm,
		Priority:  g.priority,
		Timestamp: time.Now(),
	}
	
	message := GossipMessage{
		Type:       "leadership",
		SenderID:   g.nodeID,
		Leadership: leadership,
		Timestamp:  time.Now(),
	}
	
	targets := g.selectGossipTargets(len(g.members))
	for _, target := range targets {
		go g.sendGossipMessage(target, message)
	}
}

func (g *GossipElection) resetGossipTimer() {
	if g.gossipTimer != nil {
		g.gossipTimer.Stop()
	}
	
	jitter := time.Duration(rand.Intn(int(g.gossipInterval/time.Millisecond))) * time.Millisecond / 2
	g.gossipTimer = time.NewTimer(g.gossipInterval + jitter)
}

func (g *GossipElection) updateNodeInfo() {
	nodeInfo := &storage.NodeInfo{
		ID:          g.nodeID,
		Address:     g.address,
		IsLeader:    g.isLeader,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	if err := g.storage.RegisterNode(g.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to update node info")
	}
}

func (g *GossipElection) notifyCallbacks() {
	for _, callback := range g.callbacks {
		go callback(g.isLeader, g.currentLeader)
	}
}

func (g *GossipElection) GetMembershipInfo() map[string]*GossipMember {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	members := make(map[string]*GossipMember)
	for id, member := range g.members {
		memberCopy := *member
		members[id] = &memberCopy
	}
	
	return members
}