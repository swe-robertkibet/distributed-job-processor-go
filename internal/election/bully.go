package election

import (
	"context"
	"sync"
	"time"

	"distributed-job-processor/internal/logger"
	"distributed-job-processor/internal/storage"
)

type BullyElection struct {
	nodeID     string
	nodeAddr   string
	nodes      map[string]*Node
	isLeader   bool
	storage    *storage.MongoStorage
	timeout    time.Duration
	interval   time.Duration
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	callbacks  []LeadershipCallback
}

type Node struct {
	ID       string
	Address  string
	Priority int
	LastSeen time.Time
}

type LeadershipCallback func(isLeader bool, leaderID string)

func NewBullyElection(nodeID string, storage *storage.MongoStorage, timeout, interval time.Duration) *BullyElection {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Generate dynamic address based on nodeID
	nodeAddr := generateNodeAddress(nodeID)
	
	return &BullyElection{
		nodeID:   nodeID,
		nodeAddr: nodeAddr,
		nodes:    make(map[string]*Node),
		isLeader: false,
		storage:  storage,
		timeout:  timeout,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// generateNodeAddress creates the internal Docker network address for a node
func generateNodeAddress(nodeID string) string {
	// In Docker Compose, containers can reach each other by service name
	// job-processor-1 -> job-processor-1:8080
	// job-processor-2 -> job-processor-2:8080  
	// job-processor-3 -> job-processor-3:8080
	return "job-processor-" + nodeID[len("node-"):] + ":8080"
}

func (b *BullyElection) Start() {
	logger.WithField("node_id", b.nodeID).Info("Starting Bully election algorithm")
	
	go b.runElectionLoop()
	go b.runHeartbeat()
}

func (b *BullyElection) Stop() {
	logger.Info("Stopping Bully election")
	b.cancel()
}

func (b *BullyElection) IsLeader() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isLeader
}

func (b *BullyElection) GetLeader() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	for _, node := range b.nodes {
		if node.Priority == b.getHighestPriority() {
			return node.ID
		}
	}
	
	if b.isLeader {
		return b.nodeID
	}
	
	return ""
}

func (b *BullyElection) AddCallback(callback LeadershipCallback) {
	b.callbacks = append(b.callbacks, callback)
}

func (b *BullyElection) runElectionLoop() {
	// Immediately perform initial election check
	logger.WithField("node_id", b.nodeID).Info("Performing initial election check")
	b.updateNodes()
	b.checkLeadership()
	
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.updateNodes()
			b.checkLeadership()
		}
	}
}

func (b *BullyElection) runHeartbeat() {
	// Send initial heartbeat immediately to register this node
	logger.WithField("node_id", b.nodeID).Info("Sending initial heartbeat")
	b.sendHeartbeat()
	
	ticker := time.NewTicker(b.interval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.sendHeartbeat()
		}
	}
}

func (b *BullyElection) updateNodes() {
	logger.WithField("node_id", b.nodeID).Debug("Updating nodes from storage")
	
	nodes, err := b.storage.GetNodes(b.ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to get nodes from storage")
		return
	}

	logger.WithFields(map[string]interface{}{
		"node_id": b.nodeID,
		"nodes_count": len(nodes),
	}).Debug("Retrieved nodes from storage")

	b.mu.Lock()
	defer b.mu.Unlock()

	b.nodes = make(map[string]*Node)
	for _, nodeInfo := range nodes {
		priority := b.calculatePriority(nodeInfo.ID)
		b.nodes[nodeInfo.ID] = &Node{
			ID:       nodeInfo.ID,
			Address:  nodeInfo.Address,
			Priority: priority,
			LastSeen: nodeInfo.LastSeen,
		}
		
		logger.WithFields(map[string]interface{}{
			"node_id": b.nodeID,
			"discovered_node": nodeInfo.ID,
			"address": nodeInfo.Address,
			"priority": priority,
			"last_seen": nodeInfo.LastSeen,
			"is_leader": nodeInfo.IsLeader,
		}).Debug("Discovered node")
	}
}

func (b *BullyElection) checkLeadership() {
	b.mu.Lock()
	defer b.mu.Unlock()

	myPriority := b.calculatePriority(b.nodeID)
	highestPriority := b.getHighestPriority()
	
	shouldBeLeader := myPriority >= highestPriority
	wasLeader := b.isLeader

	logger.WithFields(map[string]interface{}{
		"node_id": b.nodeID,
		"my_priority": myPriority,
		"highest_priority": highestPriority,
		"should_be_leader": shouldBeLeader,
		"is_leader": b.isLeader,
		"total_nodes": len(b.nodes),
	}).Info("Election check")

	if shouldBeLeader && !b.isLeader {
		logger.WithField("node_id", b.nodeID).Info("Becoming leader!")
		b.becomeLeader()
	} else if !shouldBeLeader && b.isLeader {
		logger.WithField("node_id", b.nodeID).Info("Stepping down from leadership")
		b.stepDown()
	}

	if wasLeader != b.isLeader {
		b.notifyCallbacks()
	}
}

func (b *BullyElection) becomeLeader() {
	b.isLeader = true
	logger.WithFields(map[string]interface{}{
		"node_id": b.nodeID,
		"address": b.nodeAddr,
	}).Info("Became leader")
	
	nodeInfo := &storage.NodeInfo{
		ID:          b.nodeID,
		Address:     b.nodeAddr,
		IsLeader:    true,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	if err := b.storage.RegisterNode(b.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to register as leader")
	}
}

func (b *BullyElection) stepDown() {
	b.isLeader = false
	logger.WithFields(map[string]interface{}{
		"node_id": b.nodeID,
		"address": b.nodeAddr,
	}).Info("Stepped down from leadership")
	
	nodeInfo := &storage.NodeInfo{
		ID:          b.nodeID,
		Address:     b.nodeAddr,
		IsLeader:    false,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	if err := b.storage.RegisterNode(b.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to update leadership status")
	}
}

func (b *BullyElection) sendHeartbeat() {
	nodeInfo := &storage.NodeInfo{
		ID:          b.nodeID,
		Address:     b.nodeAddr,
		IsLeader:    b.isLeader,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	logger.WithFields(map[string]interface{}{
		"node_id": b.nodeID,
		"address": b.nodeAddr,
		"is_leader": b.isLeader,
	}).Debug("Sending heartbeat to MongoDB")
	
	if err := b.storage.RegisterNode(b.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to send heartbeat - MongoDB connection issue")
	} else {
		logger.WithField("node_id", b.nodeID).Debug("Heartbeat sent successfully")
	}
}

func (b *BullyElection) calculatePriority(nodeID string) int {
	hash := 0
	for _, char := range nodeID {
		hash = hash*31 + int(char)
	}
	
	if hash < 0 {
		hash = -hash
	}
	
	return hash
}

func (b *BullyElection) getHighestPriority() int {
	highest := b.calculatePriority(b.nodeID)
	
	for _, node := range b.nodes {
		if node.Priority > highest && time.Since(node.LastSeen) < b.timeout {
			highest = node.Priority
		}
	}
	
	return highest
}

func (b *BullyElection) notifyCallbacks() {
	leaderID := b.GetLeader()
	for _, callback := range b.callbacks {
		go callback(b.isLeader, leaderID)
	}
}