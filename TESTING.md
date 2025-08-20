# Testing Guide for Distributed Job Processor

This comprehensive guide covers all testing approaches for the distributed job processing framework, including election algorithms, load balancing strategies, and complete system validation.

## 🚀 Quick Start

### Option 1: Docker Compose (Recommended)

```bash
# Start with default Bully algorithm
docker-compose up -d

# Test basic functionality
chmod +x examples/curl_examples.sh
./examples/curl_examples.sh

# Monitor the cluster
./scripts/monitor.sh
```

### Option 2: Automated Testing

```bash
# Run all algorithm tests
chmod +x scripts/test-algorithms.sh
./scripts/test-algorithms.sh

# Or test specific algorithm
./scripts/test-algorithms.sh 2  # Bully only
./scripts/test-algorithms.sh 3  # Raft only
./scripts/test-algorithms.sh 4  # Gossip only
```

### Option 3: Local Development

```bash
# Start local development environment
chmod +x scripts/local-dev.sh

# Single node
./scripts/local-dev.sh start-single raft

# Multi-node cluster
./scripts/local-dev.sh start-cluster gossip

# Monitor in real-time
./scripts/local-dev.sh monitor
```

## 📋 Testing Scenarios

### 1. Election Algorithm Testing

#### Bully Algorithm (2-10 nodes)

```bash
# Docker Compose
docker-compose -f docker-compose.bully.yml up -d

# Local development
./scripts/local-dev.sh start-cluster bully

# Test characteristics:
# - Fast election (~1-2s)
# - Simple priority-based selection
# - Good for stable clusters
```

#### Raft Consensus (3-7 nodes)

```bash
# Docker Compose
docker-compose -f docker-compose.raft.yml up -d

# Local development
./scripts/local-dev.sh start-cluster raft

# Test characteristics:
# - Strong consistency
# - Majority quorum required
# - Handles network partitions
```

#### Gossip Election (5+ nodes)

```bash
# Docker Compose
docker-compose -f docker-compose.gossip.yml up -d

# Local development
./scripts/local-dev.sh start-large

# Test characteristics:
# - Eventually consistent
# - Scales to large clusters
# - Built-in failure detection
```

### 2. Load Balancing Strategy Testing

#### Round Robin

```bash
LOAD_BALANCER_STRATEGY=round_robin docker-compose up -d
```

#### Least Loaded

```bash
LOAD_BALANCER_STRATEGY=least_loaded docker-compose up -d
```

#### Random Distribution

```bash
LOAD_BALANCER_STRATEGY=random docker-compose up -d
```

#### Priority-based Scheduling

```bash
LOAD_BALANCER_STRATEGY=priority docker-compose up -d
```

### 3. Failover Testing

#### Leader Failover

```bash
# Start cluster
docker-compose -f docker-compose.raft.yml up -d

# Monitor leader
watch -n 2 'curl -s http://localhost:8080/api/v1/election | jq'

# Kill current leader (in another terminal)
docker-compose -f docker-compose.raft.yml stop job-processor-1

# Observe new leader election
# Restart failed node
docker-compose -f docker-compose.raft.yml start job-processor-1
```

#### Network Partition Simulation

```bash
# Start cluster
docker-compose up -d

# Simulate network partition by stopping containers
docker-compose stop job-processor-2 job-processor-3

# Observe behavior
curl http://localhost:8080/api/v1/cluster

# Restore partition
docker-compose start job-processor-2 job-processor-3
```

### 4. Performance Testing

#### Load Testing with Jobs

```bash
# Send multiple jobs
./scripts/local-dev.sh jobs 100 8080

# Monitor processing
./scripts/monitor.sh once

# Check metrics
curl http://localhost:9090/metrics | grep jobs_total
```

#### Concurrent Client Testing

```bash
# Start multiple job creators in parallel
for i in {1..5}; do
  go run examples/client_example.go &
done

# Monitor system performance
./scripts/monitor.sh
```

### 5. Stress Testing

#### High Job Volume

```bash
# Create test script for high volume
cat << 'EOF' > stress_test.sh
#!/bin/bash
for i in {1..1000}; do
  curl -X POST http://localhost:8080/api/v1/jobs \
    -H "Content-Type: application/json" \
    -d "{\"type\":\"test\",\"priority\":$((RANDOM%10+1)),\"payload\":{\"id\":$i}}" &

  if [ $((i % 50)) -eq 0 ]; then
    wait  # Wait for batch to complete
    echo "Sent $i jobs..."
  fi
done
wait
EOF

chmod +x stress_test.sh
./stress_test.sh
```

#### Rapid Node Changes

```bash
# Script to rapidly start/stop nodes
while true; do
  docker-compose stop job-processor-2
  sleep 10
  docker-compose start job-processor-2
  sleep 10
done
```

## 🔍 Monitoring and Debugging

### Real-time Monitoring

```bash
# Interactive dashboard
./scripts/monitor.sh

# One-time status check
./scripts/monitor.sh once

# View metrics only
./scripts/monitor.sh metrics

# View logs
./scripts/monitor.sh logs
```

### Debugging Tools

```bash
# Full diagnostic report
./scripts/debug.sh

# Specific debugging
./scripts/debug.sh health      # Health check
./scripts/debug.sh election    # Election debugging
./scripts/debug.sh network     # Network connectivity
./scripts/debug.sh performance # Performance analysis
```

### API Endpoints for Testing

#### Election Information

```bash
# Current election status
curl http://localhost:8080/api/v1/election | jq

# Algorithm information
curl http://localhost:8080/api/v1/election/algorithms/raft | jq

# Cluster status
curl http://localhost:8080/api/v1/cluster | jq
```

#### Job Management

```bash
# Create job
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"test","priority":5,"payload":{"msg":"test"}}'

# List jobs
curl http://localhost:8080/api/v1/jobs | jq

# Get job by ID
curl http://localhost:8080/api/v1/jobs/{job_id} | jq
```

#### System Statistics

```bash
# Overall stats
curl http://localhost:8080/api/v1/stats | jq

# Worker status
curl http://localhost:8080/api/v1/workers | jq

# Node list
curl http://localhost:8080/api/v1/nodes | jq
```

## 🧪 Unit and Integration Tests

### Running Go Tests

```bash
# All tests
go test ./tests/...

# Specific test suites
go test ./tests/ -run TestElection
go test ./tests/ -run TestLoadBalancing
go test ./tests/ -run TestJobProcessing

# Verbose output
go test ./tests/ -v

# Run with coverage
go test ./tests/ -cover

# Benchmarks
go test ./tests/ -bench=. -benchtime=5s
```

### Test Categories

#### Unit Tests

- Load balancing algorithm logic
- Retry policy calculations
- Job state transitions
- Authentication and authorization

#### Integration Tests

- Multi-node election scenarios
- Job processing workflows
- API endpoint functionality
- Database interactions

#### Performance Tests

- Election algorithm performance
- Job processing throughput
- API response times
- Memory and CPU usage

## 📊 Expected Behaviors

### Election Convergence Times

| Algorithm | Cluster Size | Expected Time | Network Messages |
| --------- | ------------ | ------------- | ---------------- |
| Bully     | 3 nodes      | 1-3 seconds   | O(n²)            |
| Raft      | 3 nodes      | 2-5 seconds   | O(n)             |
| Gossip    | 5 nodes      | 5-15 seconds  | O(log n)         |

### Performance Baselines

#### Single Node

- **Job Throughput**: 100-500 jobs/second
- **API Response**: < 100ms
- **Memory Usage**: < 100MB
- **CPU Usage**: < 50%

#### Three Node Cluster

- **Job Throughput**: 200-1000 jobs/second
- **Election Time**: < 10 seconds
- **Failover Time**: < 30 seconds

## 🔧 Troubleshooting Common Issues

### No Leader Elected

```bash
# Check node connectivity
./scripts/debug.sh network

# Verify algorithm configuration
./scripts/debug.sh config

# Check for split brain
./scripts/debug.sh election
```

### Jobs Not Processing

```bash
# Check worker registration
curl http://localhost:8080/api/v1/workers

# Verify Redis connectivity
redis-cli ping

# Check job queue status
curl http://localhost:8080/api/v1/stats | jq '.queue'
```

### High Latency

```bash
# Check system resources
docker stats

# Analyze response times
./scripts/debug.sh performance

# Monitor network latency
ping localhost
```

### Split Brain Detection

```bash
# Check multiple leaders
for port in 8080 8081 8082; do
  echo "Port $port:"
  curl -s http://localhost:$port/api/v1/election | jq '.is_leader'
done
```

## 🎯 Test Scenarios by Use Case

### Development Testing

1. Single node with Bully algorithm
2. Basic job creation and processing
3. API endpoint validation
4. Unit test execution

### Staging Testing

1. Three node Raft cluster
2. Failover scenarios
3. Load testing with moderate volume
4. Integration test suite

### Production Simulation

1. Five node Gossip cluster
2. High availability testing
3. Stress testing with high job volume
4. Network partition scenarios
5. Security and authentication testing

## 📚 Additional Resources

### Configuration Examples

- See `examples/election_algorithms.md` for detailed configuration
- Check `docker-compose.*.yml` files for deployment examples
- Review `.env` file for environment variables

### Monitoring

- Grafana dashboards: http://localhost:3000 (admin/admin)
- Prometheus metrics: http://localhost:9090
- Application logs: `docker-compose logs -f`

### API Documentation

- Health endpoint: `GET /health`
- Election status: `GET /api/v1/election`
- Job management: `POST /api/v1/jobs`
- System stats: `GET /api/v1/stats`
