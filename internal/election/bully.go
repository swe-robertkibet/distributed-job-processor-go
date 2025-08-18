package election

import (
	"context"
	"sync"
	"time"

	"distributed-job-processor/internal/logger"
	"distributed-job-processor/internal/storage"

	"github.com/sirupsen/logrus"
)

type BullyElection struct {
	nodeID     string
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
	
	return &BullyElection{
		nodeID:   nodeID,
		nodes:    make(map[string]*Node),
		isLeader: false,
		storage:  storage,
		timeout:  timeout,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
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
	nodes, err := b.storage.GetNodes(b.ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to get nodes from storage")
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.nodes = make(map[string]*Node)
	for _, nodeInfo := range nodes {
		b.nodes[nodeInfo.ID] = &Node{
			ID:       nodeInfo.ID,
			Address:  nodeInfo.Address,
			Priority: b.calculatePriority(nodeInfo.ID),
			LastSeen: nodeInfo.LastSeen,
		}
	}
}

func (b *BullyElection) checkLeadership() {
	b.mu.Lock()
	defer b.mu.Unlock()

	myPriority := b.calculatePriority(b.nodeID)
	highestPriority := b.getHighestPriority()
	
	shouldBeLeader := myPriority >= highestPriority
	wasLeader := b.isLeader

	if shouldBeLeader && !b.isLeader {
		b.becomeLeader()
	} else if !shouldBeLeader && b.isLeader {
		b.stepDown()
	}

	if wasLeader != b.isLeader {
		b.notifyCallbacks()
	}
}

func (b *BullyElection) becomeLeader() {
	b.isLeader = true
	logger.WithField("node_id", b.nodeID).Info("Became leader")
	
	nodeInfo := &storage.NodeInfo{
		ID:          b.nodeID,
		Address:     "localhost:8080",
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
	logger.WithField("node_id", b.nodeID).Info("Stepped down from leadership")
	
	nodeInfo := &storage.NodeInfo{
		ID:          b.nodeID,
		Address:     "localhost:8080",
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
		Address:     "localhost:8080",
		IsLeader:    b.isLeader,
		WorkerCount: 10,
		Version:     "1.0.0",
	}
	
	if err := b.storage.RegisterNode(b.ctx, nodeInfo); err != nil {
		logger.WithError(err).Error("Failed to send heartbeat")
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