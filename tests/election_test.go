package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"distributed-job-processor/internal/config"
	"distributed-job-processor/internal/election"
	"distributed-job-processor/internal/storage"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorage(t *testing.T) (*storage.MongoStorage, func()) {
	// Load .env file from parent directory (tests run from ./tests directory)
	err := godotenv.Load("../.env")
	if err != nil {
		t.Logf("Warning: Could not load .env file: %v", err)
	}
	
	// Load configuration from environment (including .env file)
	fullCfg, err := config.Load()
	if err != nil {
		t.Skipf("Failed to load configuration: %v", err)
		return nil, nil
	}

	// Use test database name
	testDatabase := "election_test"
	
	mongoStorage, err := storage.NewMongoStorage(fullCfg.MongoDB.URI, testDatabase, fullCfg.MongoDB.Timeout)
	if err != nil {
		t.Skipf("MongoDB Atlas not available for testing: %v", err)
		return nil, nil
	}

	cleanup := func() {
		// Clear test data before closing
		if mongoStorage != nil {
			ctx := context.Background()
			mongoStorage.ClearAllNodes(ctx) // Clear all node registrations
		}
		mongoStorage.Close()
	}

	return mongoStorage, cleanup
}

func TestElectionFactory(t *testing.T) {
	factory := election.NewElectionFactory()

	supportedAlgorithms := factory.GetSupportedAlgorithms()
	expectedAlgorithms := []string{"bully", "raft", "gossip"}

	assert.ElementsMatch(t, expectedAlgorithms, supportedAlgorithms)

	for _, algorithm := range expectedAlgorithms {
		t.Run(algorithm, func(t *testing.T) {
			description := factory.GetAlgorithmDescription(algorithm)
			assert.NotEqual(t, "Unknown algorithm", description)
			assert.NotEmpty(t, description)

			config := factory.GetRecommendedConfiguration(algorithm, 5)
			assert.Contains(t, config, "election_timeout")
			assert.Contains(t, config, "election_interval")
			assert.Contains(t, config, "recommended_for")
			assert.Contains(t, config, "characteristics")
		})
	}
}

func TestElectionFactoryCreation(t *testing.T) {
	mongoStorage, cleanup := setupTestStorage(t)
	if mongoStorage == nil {
		return
	}
	defer cleanup()

	factory := election.NewElectionFactory()
	nodeID := "test-node"
	timeout := 5 * time.Second
	interval := 2 * time.Second

	testCases := []struct {
		algorithm string
		expectErr bool
	}{
		{"bully", false},
		{"raft", false},
		{"gossip", false},
		{"unknown", true},
	}

	for _, tc := range testCases {
		t.Run(tc.algorithm, func(t *testing.T) {
			election, err := factory.CreateElection(tc.algorithm, nodeID, mongoStorage, timeout, interval)

			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, election)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, election)

				assert.False(t, election.IsLeader())
				assert.Equal(t, "", election.GetLeader())

				election.Stop()
			}
		})
	}
}

func TestBullyElection(t *testing.T) {
	mongoStorage, cleanup := setupTestStorage(t)
	if mongoStorage == nil {
		return
	}
	defer cleanup()

	bullyElection := election.NewBullyElection("test-node", mongoStorage, 2*time.Second, 1*time.Second)

	callbackCalled := false
	var callbackIsLeader bool
	var callbackLeaderID string

	bullyElection.AddCallback(func(isLeader bool, leaderID string) {
		callbackCalled = true
		callbackIsLeader = isLeader
		callbackLeaderID = leaderID
	})

	assert.False(t, bullyElection.IsLeader())
	assert.Equal(t, "", bullyElection.GetLeader())

	bullyElection.Start()
	time.Sleep(100 * time.Millisecond)

	bullyElection.Stop()

	if callbackCalled {
		assert.Equal(t, callbackIsLeader, bullyElection.IsLeader())
		assert.Equal(t, callbackLeaderID, bullyElection.GetLeader())
	}
}

func TestRaftElection(t *testing.T) {
	mongoStorage, cleanup := setupTestStorage(t)
	if mongoStorage == nil {
		return
	}
	defer cleanup()

	raftElection := election.NewRaftElection("test-node", mongoStorage, 2*time.Second, 500*time.Millisecond)

	callbackCalled := false
	var callbackIsLeader bool
	var callbackLeaderID string

	raftElection.AddCallback(func(isLeader bool, leaderID string) {
		callbackCalled = true
		callbackIsLeader = isLeader
		callbackLeaderID = leaderID
	})

	assert.False(t, raftElection.IsLeader())
	assert.Equal(t, "", raftElection.GetLeader())

	raftElection.Start()
	time.Sleep(100 * time.Millisecond)

	raftElection.Stop()

	if callbackCalled {
		assert.Equal(t, callbackIsLeader, raftElection.IsLeader())
		assert.Equal(t, callbackLeaderID, raftElection.GetLeader())
	}
}

func TestGossipElection(t *testing.T) {
	mongoStorage, cleanup := setupTestStorage(t)
	if mongoStorage == nil {
		return
	}
	defer cleanup()

	gossipElection := election.NewGossipElection("test-node", mongoStorage, 1*time.Second, 3*time.Second)

	callbackCalled := false
	var callbackIsLeader bool
	var callbackLeaderID string

	gossipElection.AddCallback(func(isLeader bool, leaderID string) {
		callbackCalled = true
		callbackIsLeader = isLeader
		callbackLeaderID = leaderID
	})

	assert.False(t, gossipElection.IsLeader())
	assert.Equal(t, "", gossipElection.GetLeader())

	gossipElection.Start()
	time.Sleep(100 * time.Millisecond)

	gossipElection.Stop()

	if callbackCalled {
		assert.Equal(t, callbackIsLeader, gossipElection.IsLeader())
		assert.Equal(t, callbackLeaderID, gossipElection.GetLeader())
	}
}

func TestMultipleNodeElection(t *testing.T) {
	mongoStorage, cleanup := setupTestStorage(t)
	if mongoStorage == nil {
		return
	}
	defer cleanup()

	algorithms := []string{"bully", "raft", "gossip"}

	for _, algorithm := range algorithms {
		t.Run(algorithm, func(t *testing.T) {
			// Clean any existing node data before starting this algorithm test
			ctx := context.Background()
			mongoStorage.ClearAllNodes(ctx)
			
			factory := election.NewElectionFactory()

			nodes := make([]election.Election, 3)
			for i := 0; i < 3; i++ {
				nodeID := fmt.Sprintf("node-%d", i)
				election, err := factory.CreateElection(algorithm, nodeID, mongoStorage, 2*time.Second, 1*time.Second)
				require.NoError(t, err)
				nodes[i] = election
			}

			for _, node := range nodes {
				node.Start()
			}

			time.Sleep(3 * time.Second)

			leaderCount := 0
			var leaderID string
			for _, node := range nodes {
				if node.IsLeader() {
					leaderCount++
					leaderID = node.GetLeader()
				}
			}

			for _, node := range nodes {
				if node.GetLeader() != "" {
					assert.Equal(t, leaderID, node.GetLeader(), "All nodes should agree on leader")
				}
			}

			for _, node := range nodes {
				node.Stop()
			}
			
			// Give time for all nodes to fully stop and release connections
			time.Sleep(200 * time.Millisecond)
			
			// Clean up after this algorithm test
			cleanupCtx := context.Background()
			mongoStorage.ClearAllNodes(cleanupCtx)
			
			// Give time for cleanup to complete before next test
			time.Sleep(300 * time.Millisecond)

			if algorithm == "raft" {
				assert.LessOrEqual(t, leaderCount, 1, "Should have at most one leader in Raft")
			}
		})
	}
}

func TestElectionFailover(t *testing.T) {
	mongoStorage, cleanup := setupTestStorage(t)
	if mongoStorage == nil {
		return
	}
	defer cleanup()

	factory := election.NewElectionFactory()

	node1, err := factory.CreateElection("bully", "node-1", mongoStorage, 1*time.Second, 500*time.Millisecond)
	require.NoError(t, err)

	node2, err := factory.CreateElection("bully", "node-2", mongoStorage, 1*time.Second, 500*time.Millisecond)
	require.NoError(t, err)

	node1.Start()
	node2.Start()

	time.Sleep(2 * time.Second)

	initialLeader := node1.GetLeader()
	if initialLeader == "" {
		initialLeader = node2.GetLeader()
	}

	if initialLeader == "node-1" {
		node1.Stop()
	} else {
		node2.Stop()
	}

	time.Sleep(2 * time.Second)

	var remainingNode election.Election
	if initialLeader == "node-1" {
		remainingNode = node2
	} else {
		remainingNode = node1
	}

	newLeader := remainingNode.GetLeader()
	assert.NotEqual(t, initialLeader, newLeader, "Leader should change after failover")

	if initialLeader == "node-1" {
		node1.Stop()
	} else {
		node2.Stop()
	}
	remainingNode.Stop()
}

func BenchmarkElectionCreation(b *testing.B) {
	mongoStorage, cleanup := setupTestStorage(&testing.T{})
	if mongoStorage == nil {
		b.Skip("MongoDB not available for benchmarking")
		return
	}
	defer cleanup()

	factory := election.NewElectionFactory()
	algorithms := []string{"bully", "raft", "gossip"}

	for _, algorithm := range algorithms {
		b.Run(algorithm, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				nodeID := fmt.Sprintf("node-%d", i)
				election, err := factory.CreateElection(algorithm, nodeID, mongoStorage, 5*time.Second, 2*time.Second)
				if err != nil {
					b.Fatal(err)
				}
				election.Stop()
			}
		})
	}
}

func BenchmarkElectionOperations(b *testing.B) {
	mongoStorage, cleanup := setupTestStorage(&testing.T{})
	if mongoStorage == nil {
		b.Skip("MongoDB not available for benchmarking")
		return
	}
	defer cleanup()

	factory := election.NewElectionFactory()
	election, err := factory.CreateElection("bully", "bench-node", mongoStorage, 5*time.Second, 2*time.Second)
	if err != nil {
		b.Fatal(err)
	}
	defer election.Stop()

	election.Start()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		election.IsLeader()
		election.GetLeader()
	}
}