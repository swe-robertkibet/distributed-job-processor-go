package election

import (
	"fmt"
	"time"

	"distributed-job-processor/internal/storage"
)

type Election interface {
	Start()
	Stop()
	IsLeader() bool
	GetLeader() string
	AddCallback(callback LeadershipCallback)
}

type ElectionFactory struct{}

func NewElectionFactory() *ElectionFactory {
	return &ElectionFactory{}
}

func (f *ElectionFactory) CreateElection(
	algorithm string,
	nodeID string,
	storage *storage.MongoStorage,
	timeout time.Duration,
	interval time.Duration,
) (Election, error) {
	switch algorithm {
	case "bully":
		return NewBullyElection(nodeID, storage, timeout, interval), nil
		
	case "raft":
		heartbeatTimeout := interval / 3
		if heartbeatTimeout < 50*time.Millisecond {
			heartbeatTimeout = 50 * time.Millisecond
		}
		return NewRaftElection(nodeID, storage, timeout, heartbeatTimeout), nil
		
	case "gossip":
		gossipInterval := interval / 2
		if gossipInterval < 100*time.Millisecond {
			gossipInterval = 100 * time.Millisecond
		}
		suspicionTimeout := timeout / 2
		if suspicionTimeout < 1*time.Second {
			suspicionTimeout = 1 * time.Second
		}
		return NewGossipElection(nodeID, storage, gossipInterval, suspicionTimeout), nil
		
	default:
		return nil, fmt.Errorf("unknown election algorithm: %s", algorithm)
	}
}

func (f *ElectionFactory) GetSupportedAlgorithms() []string {
	return []string{"bully", "raft", "gossip"}
}

func (f *ElectionFactory) GetAlgorithmDescription(algorithm string) string {
	descriptions := map[string]string{
		"bully": "Bully Algorithm - Simple leader election based on node priorities. " +
			"Best for small clusters with stable membership. " +
			"Leader is the node with highest priority that's currently alive.",
			
		"raft": "Raft Consensus - Strong consistency with leader election and log replication. " +
			"Best for critical systems requiring consensus on state changes. " +
			"Handles network partitions gracefully with majority quorum.",
			
		"gossip": "Gossip-based Election - Decentralized membership management with failure detection. " +
			"Best for large, dynamic clusters with frequent membership changes. " +
			"Eventually consistent leader election through gossip protocol.",
	}
	
	if desc, exists := descriptions[algorithm]; exists {
		return desc
	}
	
	return "Unknown algorithm"
}

func (f *ElectionFactory) GetRecommendedConfiguration(algorithm string, clusterSize int) map[string]interface{} {
	switch algorithm {
	case "bully":
		return map[string]interface{}{
			"election_timeout": "10s",
			"election_interval": "30s",
			"recommended_for": "Small clusters (2-10 nodes)",
			"characteristics": []string{
				"Simple and fast",
				"Single point of failure detection",
				"Good for stable membership",
			},
		}
		
	case "raft":
		electionTimeout := "5s"
		if clusterSize > 5 {
			electionTimeout = "10s"
		}
		
		return map[string]interface{}{
			"election_timeout": electionTimeout,
			"election_interval": "2s",
			"recommended_for": "Medium clusters (3-7 nodes)",
			"characteristics": []string{
				"Strong consistency guarantees",
				"Handles network partitions",
				"Requires majority quorum",
			},
		}
		
	case "gossip":
		gossipInterval := "1s"
		if clusterSize > 20 {
			gossipInterval = "2s"
		}
		
		return map[string]interface{}{
			"election_timeout": "15s",
			"election_interval": gossipInterval,
			"recommended_for": "Large clusters (10+ nodes)",
			"characteristics": []string{
				"Scales to large clusters",
				"Handles dynamic membership",
				"Eventually consistent",
				"Built-in failure detection",
			},
		}
		
	default:
		return map[string]interface{}{
			"error": "Unknown algorithm",
		}
	}
}