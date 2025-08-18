# Election Algorithm Examples

This document demonstrates how to configure and use different election algorithms in the distributed job processor.

## Available Algorithms

### 1. Bully Algorithm (Default)
Simple leader election based on node priorities.

**Configuration:**
```bash
ELECTION_ALGORITHM=bully
ELECTION_TIMEOUT=10s
ELECTION_INTERVAL=30s
```

**Best for:**
- Small clusters (2-10 nodes)
- Stable membership
- Simple deployment scenarios

**Characteristics:**
- Simple and fast
- Single point of failure detection
- Good for stable membership

### 2. Raft Consensus
Strong consistency with leader election and log replication.

**Configuration:**
```bash
ELECTION_ALGORITHM=raft
ELECTION_TIMEOUT=5s
ELECTION_INTERVAL=2s
```

**Best for:**
- Medium clusters (3-7 nodes)
- Critical systems requiring consensus
- Applications needing strong consistency

**Characteristics:**
- Strong consistency guarantees
- Handles network partitions
- Requires majority quorum
- Built-in log replication

### 3. Gossip-based Election
Decentralized membership management with failure detection.

**Configuration:**
```bash
ELECTION_ALGORITHM=gossip
ELECTION_TIMEOUT=15s
ELECTION_INTERVAL=1s
```

**Best for:**
- Large clusters (10+ nodes)
- Dynamic membership
- High availability scenarios

**Characteristics:**
- Scales to large clusters
- Handles dynamic membership
- Eventually consistent
- Built-in failure detection

## Testing Different Algorithms

### Docker Compose Examples

#### Bully Algorithm Cluster
```yaml
version: '3.8'
services:
  job-processor-1:
    build: .
    environment:
      - ELECTION_ALGORITHM=bully
      - NODE_ID=node-1
      - ELECTION_TIMEOUT=10s
    
  job-processor-2:
    build: .
    environment:
      - ELECTION_ALGORITHM=bully
      - NODE_ID=node-2
      - ELECTION_TIMEOUT=10s
```

#### Raft Consensus Cluster
```yaml
version: '3.8'
services:
  job-processor-1:
    build: .
    environment:
      - ELECTION_ALGORITHM=raft
      - NODE_ID=node-1
      - ELECTION_TIMEOUT=5s
      - ELECTION_INTERVAL=2s
    
  job-processor-2:
    build: .
    environment:
      - ELECTION_ALGORITHM=raft
      - NODE_ID=node-2
      - ELECTION_TIMEOUT=5s
      - ELECTION_INTERVAL=2s
```

#### Gossip-based Cluster
```yaml
version: '3.8'
services:
  job-processor-1:
    build: .
    environment:
      - ELECTION_ALGORITHM=gossip
      - NODE_ID=node-1
      - ELECTION_TIMEOUT=15s
      - ELECTION_INTERVAL=1s
    
  job-processor-2:
    build: .
    environment:
      - ELECTION_ALGORITHM=gossip
      - NODE_ID=node-2
      - ELECTION_TIMEOUT=15s
      - ELECTION_INTERVAL=1s
```

## API Endpoints for Election Information

### Get Current Election Status
```bash
curl http://localhost:8080/api/v1/election
```

Response:
```json
{
  "current_algorithm": "raft",
  "is_leader": true,
  "leader_id": "node-1",
  "node_id": "node-1",
  "supported_algorithms": ["bully", "raft", "gossip"]
}
```

### Get Algorithm Information
```bash
curl http://localhost:8080/api/v1/election/algorithms/raft
```

Response:
```json
{
  "algorithm": "raft",
  "description": "Raft Consensus - Strong consistency with leader election and log replication...",
  "configuration": {
    "election_timeout": "5s",
    "election_interval": "2s",
    "recommended_for": "Medium clusters (3-7 nodes)",
    "characteristics": [
      "Strong consistency guarantees",
      "Handles network partitions",
      "Requires majority quorum"
    ]
  }
}
```

### Get Cluster Status
```bash
curl http://localhost:8080/api/v1/cluster
```

Response:
```json
{
  "cluster_size": 3,
  "leader": {
    "id": "node-1",
    "address": "localhost:8080",
    "is_leader": true,
    "worker_count": 10
  },
  "election_algorithm": "raft",
  "nodes": [...]
}
```

## Performance Comparison

| Algorithm | Cluster Size | Election Time | Network Messages | Consistency |
|-----------|--------------|---------------|------------------|-------------|
| Bully     | 2-10 nodes   | ~1s          | O(n²)            | Eventually  |
| Raft      | 3-7 nodes    | ~2-5s        | O(n)             | Strong      |
| Gossip    | 10+ nodes    | ~5-15s       | O(log n)         | Eventually  |

## Choosing the Right Algorithm

### Use Bully when:
- You have a small, stable cluster
- You need simple, fast leader election
- Network is reliable
- You don't need strong consistency

### Use Raft when:
- You need strong consistency guarantees
- You're building a critical system
- You can tolerate higher latency for consistency
- You have 3-7 nodes

### Use Gossip when:
- You have a large cluster (10+ nodes)
- Nodes frequently join/leave
- You need high availability
- You can tolerate eventual consistency

## Monitoring Election Behavior

Use Prometheus metrics to monitor election behavior:

```bash
# Election events
curl http://localhost:9090/metrics | grep leader_elections_total

# Node status
curl http://localhost:9090/metrics | grep node_info
```

## Troubleshooting

### Common Issues

1. **Split Brain (Bully/Gossip)**: Ensure proper network connectivity
2. **No Quorum (Raft)**: Ensure majority of nodes are running
3. **Slow Convergence (Gossip)**: Tune gossip interval for your network
4. **Frequent Elections (All)**: Check network stability and timeout values