# Distributed Job Processor

A high-performance, fault-tolerant distributed job processing system built in Go. This isn't your typical job queue—it's a sophisticated distributed system that implements multiple consensus algorithms, advanced load balancing strategies, and enterprise-grade monitoring capabilities.

## What Makes This Special?

This system was built to solve real distributed computing challenges. While most job processors are simple single-node affairs, this one handles the complexities of distributed coordination, leader election, and fault tolerance that you'd encounter in production environments.

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Node 1        │    │   Node 2        │    │   Node 3        │
│  (Leader)       │◄──►│                 │◄──►│                 │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │  Workers    │ │    │ │  Workers    │ │    │ │  Workers    │ │
│ │  ├─Worker-1 │ │    │ │  ├─Worker-1 │ │    │ │  ├─Worker-1 │ │
│ │  ├─Worker-2 │ │    │ │  ├─Worker-2 │ │    │ │  ├─Worker-2 │ │
│ │  └─Worker-N │ │    │ │  └─Worker-N │ │    │ │  └─Worker-N │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────┐
                    │   Redis Queue   │
                    │ ┌─────────────┐ │
                    │ │ Jobs Queue  │ │
                    │ │ Processing  │ │
                    │ │ Completed   │ │
                    │ │ Failed      │ │
                    │ │ Delayed     │ │
                    │ └─────────────┘ │
                    └─────────────────┘
                             │
                    ┌─────────────────┐
                    │   MongoDB       │
                    │ ┌─────────────┐ │
                    │ │ Jobs        │ │
                    │ │ Nodes       │ │
                    │ │ Elections   │ │
                    │ │ Metrics     │ │
                    │ └─────────────┘ │
                    └─────────────────┘
```

## 🚀 Features

### Core Distributed System Features
- **Multi-Node Architecture**: True distributed processing across multiple nodes
- **Leader Election**: Three consensus algorithms (Bully, Raft, Gossip)
- **Fault Tolerance**: Automatic failover and cluster reconfiguration
- **Dynamic Membership**: Nodes can join/leave the cluster seamlessly

### Job Processing Engine
- **Priority-Based Scheduling**: High, Normal, Low priority job execution
- **Smart Retry Logic**: Exponential backoff with jitter to prevent thundering herd
- **Delayed Job Execution**: Schedule jobs for future execution
- **Job State Tracking**: Complete lifecycle management (pending → running → completed/failed)

### Load Balancing Strategies
- **Round Robin**: Fair distribution across workers
- **Least Loaded**: Route to workers with fewest active jobs
- **Random**: Distribute jobs randomly for maximum throughput
- **Priority-Based**: Match high-priority jobs to least-loaded workers

### Security & Authentication
- **JWT-Based Authentication**: Role-based access control (Admin, User, Worker)
- **TLS Encryption**: End-to-end encrypted communication
- **Configurable Security**: Can be disabled for development environments

### Monitoring & Observability
- **Prometheus Metrics**: Comprehensive system metrics collection
- **Grafana Dashboards**: Pre-configured visualization dashboards
- **Health Checks**: Built-in health endpoints for orchestration
- **Real-time Statistics**: Live job queue and worker statistics

### Developer Experience
- **Hot Configuration**: Environment variable-based configuration
- **Docker Ready**: Multi-service Docker Compose setups
- **Comprehensive Testing**: Unit, integration, and benchmark tests
- **Client Libraries**: Go client example with HTTP API

## 🏁 Quick Start

### Prerequisites
- Docker & Docker Compose
- MongoDB (or use Docker Compose)
- Redis (or use Docker Compose)

### 1. Clone and Setup
```bash
git clone <repository-url>
cd distributed-job-processor-go
```

### 2. Environment Configuration
Copy the example configuration file and customize it:
```bash
cp .env.example .env
# Edit .env with your preferred settings
```

The `.env.example` file contains all available configuration options with detailed explanations. Here are the key sections:

#### **📋 Configuration File Structure**
The configuration is organized into logical sections:

**🖥️ Server Configuration**
```bash
SERVER_PORT=8080              # HTTP API port
SERVER_HOST=0.0.0.0          # Bind address (0.0.0.0 = all interfaces)
WORKER_COUNT=10              # Worker threads per node
NODE_ID=node-1               # Unique node identifier (MUST be unique!)
```

**🗄️ Database Configuration**
```bash
# MongoDB (persistent job storage)
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=jobprocessor
MONGODB_TIMEOUT=30s

# Redis (job queue)
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=                # Leave empty if no password
REDIS_DB=0                    # Use different numbers for environments
```

**🗳️ Election Algorithm Settings**
```bash
ELECTION_ALGORITHM=bully      # bully, raft, or gossip
ELECTION_TIMEOUT=10s         # How long to wait for election
ELECTION_INTERVAL=30s        # Heartbeat frequency
```

**⚖️ Load Balancing Configuration**
```bash
LOAD_BALANCER_STRATEGY=round_robin  # round_robin, least_loaded, random, priority
```

**🔄 Retry Policy Settings**
```bash
RETRY_POLICY=exponential     # fixed or exponential
MAX_RETRIES=3               # Maximum retry attempts
RETRY_BASE_DELAY=1s         # Initial retry delay
RETRY_MAX_DELAY=60s         # Maximum retry delay
RETRY_MULTIPLIER=2.0        # Exponential backoff multiplier
RETRY_JITTER_FACTOR=0.1     # Randomness to prevent thundering herd
```

**🔒 Security Configuration**
```bash
AUTH_ENABLED=false          # Enable JWT authentication
JWT_SECRET=your-secret-key  # Change this in production!
TLS_ENABLED=false          # Enable TLS encryption
TLS_CERT_FILE=             # Path to certificate file
TLS_KEY_FILE=              # Path to private key file
```

**📊 Monitoring Settings**
```bash
METRICS_ENABLED=true       # Enable Prometheus metrics
METRICS_PORT=9090         # Metrics endpoint port
```

#### **🏭 Environment-Specific Examples**

**Development Setup**
```bash
AUTH_ENABLED=false
TLS_ENABLED=false
WORKER_COUNT=5
ELECTION_TIMEOUT=5s
```

**Production Setup**
```bash
AUTH_ENABLED=true
TLS_ENABLED=true
WORKER_COUNT=20
JWT_SECRET=your-secure-production-secret-generated-with-openssl
TLS_CERT_FILE=/etc/ssl/certs/app.crt
TLS_KEY_FILE=/etc/ssl/private/app.key
MONGODB_URI=mongodb+srv://user:pass@prod-cluster.mongodb.net
```

#### **🎯 Algorithm-Specific Configurations**

**Small Cluster (Bully Algorithm)**
```bash
ELECTION_ALGORITHM=bully
ELECTION_TIMEOUT=10s
ELECTION_INTERVAL=30s
# Best for: 2-10 stable nodes
```

**Medium Cluster (Raft Consensus)**
```bash
ELECTION_ALGORITHM=raft
ELECTION_TIMEOUT=5s
ELECTION_INTERVAL=2s
# Best for: 3-7 nodes requiring strong consistency
```

**Large Cluster (Gossip Protocol)**
```bash
ELECTION_ALGORITHM=gossip
ELECTION_TIMEOUT=15s
ELECTION_INTERVAL=1s
# Best for: 10+ nodes with dynamic membership
```

> **💡 Pro Tip**: Start with the default configuration and adjust based on your specific needs. The system is designed to work well out of the box with sensible defaults.

### 3. Start the Cluster
```bash
# For Bully Algorithm (2 nodes)
docker-compose up

# For Raft Algorithm (3 nodes)
docker-compose -f docker-compose.raft.yml up

# For Gossip Algorithm (3 nodes)
docker-compose -f docker-compose.gossip.yml up
```

### 4. Test the System
```bash
# Create a job
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email",
    "priority": 5,
    "payload": {
      "recipient": "user@example.com",
      "subject": "Test Job"
    },
    "max_retries": 3
  }'

# Check system status
curl http://localhost:8080/api/v1/stats

# View health status
curl http://localhost:8080/health
```

## 🧠 Consensus Algorithms

This system implements three different consensus algorithms, each optimized for different scenarios:

### Bully Algorithm
**Best for**: Small, stable clusters (2-10 nodes)
- Simple priority-based leader election
- Fast convergence with stable membership
- Lower overhead for small clusters

```bash
# Configuration
ELECTION_ALGORITHM=bully
ELECTION_TIMEOUT=10s
ELECTION_INTERVAL=30s
```

### Raft Consensus
**Best for**: Medium clusters requiring strong consistency (3-7 nodes)
- Industry-standard consensus protocol
- Strong consistency guarantees
- Handles network partitions gracefully
- Requires majority quorum

```bash
# Configuration
ELECTION_ALGORITHM=raft
ELECTION_TIMEOUT=5s
ELECTION_INTERVAL=2s
```

### Gossip Protocol
**Best for**: Large, dynamic clusters (10+ nodes)
- Scalable to hundreds of nodes
- Eventually consistent
- Built-in failure detection
- Handles network partitions well

```bash
# Configuration
ELECTION_ALGORITHM=gossip
ELECTION_TIMEOUT=15s
ELECTION_INTERVAL=1s
```

## ⚙️ Configuration Reference

### Server Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | HTTP server port |
| `SERVER_HOST` | `0.0.0.0` | Server bind address |
| `WORKER_COUNT` | `10` | Number of worker threads per node |
| `NODE_ID` | `node-1` | Unique identifier for this node |

### Database Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `MONGODB_URI` | `mongodb://localhost:27017` | MongoDB connection string |
| `MONGODB_DATABASE` | `jobprocessor` | Database name |
| `MONGODB_TIMEOUT` | `30s` | Connection timeout |
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | `` | Redis password (if required) |
| `REDIS_DB` | `0` | Redis database number |

### Election Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `ELECTION_ALGORITHM` | `bully` | Algorithm: `bully`, `raft`, `gossip` |
| `ELECTION_TIMEOUT` | `10s` | Election timeout duration |
| `ELECTION_INTERVAL` | `30s` | Heartbeat/election interval |

### Retry Policy Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `RETRY_POLICY` | `exponential` | Policy: `fixed`, `exponential` |
| `MAX_RETRIES` | `3` | Maximum retry attempts |
| `RETRY_BASE_DELAY` | `1s` | Initial retry delay |
| `RETRY_MAX_DELAY` | `60s` | Maximum retry delay |
| `RETRY_MULTIPLIER` | `2.0` | Exponential backoff multiplier |
| `RETRY_JITTER_FACTOR` | `0.1` | Jitter factor (0.0-1.0) |

### Security Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_ENABLED` | `false` | Enable JWT authentication |
| `JWT_SECRET` | `your-secret-key` | JWT signing secret |
| `TLS_ENABLED` | `false` | Enable TLS encryption |
| `TLS_CERT_FILE` | `` | TLS certificate file path |
| `TLS_KEY_FILE` | `` | TLS private key file path |

## 📡 API Reference

### Job Management

#### Create Job
```http
POST /api/v1/jobs
Content-Type: application/json

{
  "type": "email",
  "priority": 5,
  "payload": {
    "recipient": "user@example.com",
    "subject": "Hello World"
  },
  "max_retries": 3,
  "scheduled_at": "2024-01-01T12:00:00Z"
}
```

#### Get Job
```http
GET /api/v1/jobs/{job_id}
```

#### List Jobs
```http
GET /api/v1/jobs?status=pending&type=email&limit=50
```

#### Delete Job (Admin only)
```http
DELETE /api/v1/jobs/{job_id}
```

### System Information

#### System Statistics
```http
GET /api/v1/stats
```

#### Worker Information (Admin only)
```http
GET /api/v1/workers
```

#### Cluster Nodes (Admin only)
```http
GET /api/v1/nodes
```

#### Leader Information
```http
GET /api/v1/leader
```

#### Election Algorithm Info
```http
GET /api/v1/election/algorithms/{algorithm}
```

### Health & Monitoring

#### Health Check
```http
GET /health
```

#### Prometheus Metrics
```http
GET /metrics  # Port 9090
```

### Authentication

#### Login
```http
POST /auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}
```

**Default Users:**
- `admin` / `admin123` (Admin role)
- `user` / `user123` (User role)  
- `worker` / `worker123` (Worker role)

## 🐳 Deployment

### Development Setup
```bash
# Single node development
go run cmd/server/main.go

# Or with Docker
docker build -t job-processor .
docker run -p 8080:8080 -p 9090:9090 job-processor
```

### Production Cluster Deployments

#### 2-Node Bully Cluster
```bash
docker-compose up
# Nodes: localhost:8080, localhost:8081
```

#### 3-Node Raft Cluster
```bash
docker-compose -f docker-compose.raft.yml up
# Nodes: localhost:8080, localhost:8081, localhost:8082
```

#### 3-Node Gossip Cluster
```bash
docker-compose -f docker-compose.gossip.yml up
# Nodes: localhost:8080, localhost:8081, localhost:8082
```

### Monitoring Stack
All deployments include:
- **Prometheus**: `http://localhost:9093` (or 9092)
- **Grafana**: `http://localhost:3000` (admin/admin)
- **Redis**: `localhost:6379`

## 🧪 Testing

### Running Tests
```bash
# Unit tests
go test ./tests -v

# Integration tests (requires MongoDB & Redis)
go test ./tests -v -tags=integration

# Benchmarks
go test ./tests -bench=. -benchmem

# Election algorithm tests
go test ./tests -run=TestElection -v
```

### Test Coverage
- **Unit Tests**: Load balancing, retry policies, job processing
- **Integration Tests**: Full system workflows, API endpoints
- **Election Tests**: All three consensus algorithms
- **Benchmarks**: Performance testing for critical paths

### Sample Benchmark Results
```
BenchmarkRoundRobinSelection-8        1000000    1043 ns/op      0 B/op    0 allocs/op
BenchmarkLeastLoadedSelection-8        500000    2856 ns/op      0 B/op    0 allocs/op
BenchmarkRetryPolicyCalculation-8    5000000     267 ns/op      0 B/op    0 allocs/op
```

## 🔧 Development

### Project Structure
```
├── cmd/server/          # Main application entry point
├── pkg/job/             # Job definitions and interfaces
├── internal/
│   ├── auth/           # Authentication & authorization
│   ├── config/         # Configuration management
│   ├── election/       # Consensus algorithms
│   ├── loadbalancer/   # Load balancing strategies
│   ├── logger/         # Structured logging
│   ├── metrics/        # Prometheus metrics
│   ├── queue/          # Redis queue implementation
│   ├── retry/          # Retry policy implementations
│   ├── server/         # HTTP server & API handlers
│   ├── storage/        # MongoDB storage layer
│   ├── tls/           # TLS configuration
│   └── worker/        # Worker pool management
├── tests/              # Test suites
├── examples/           # Client examples
├── scripts/            # Development scripts
└── monitoring/         # Grafana dashboards & Prometheus config
```

### Adding Custom Job Processors
```go
type EmailProcessor struct{}

func (e *EmailProcessor) Process(ctx context.Context, job *job.Job) error {
    recipient := job.Payload["recipient"].(string)
    subject := job.Payload["subject"].(string)
    
    // Send email logic here
    return sendEmail(recipient, subject)
}

func (e *EmailProcessor) Type() string {
    return "email"
}

// Register the processor
server.RegisterProcessor(&EmailProcessor{})
```

### Contributing
1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Add tests** for your changes
4. **Commit** your changes (`git commit -m 'Add amazing feature'`)
5. **Push** to the branch (`git push origin feature/amazing-feature`)
6. **Open** a Pull Request

### Performance Considerations
- The system is optimized for throughput over latency
- Raft algorithm provides strongest consistency but highest overhead
- Gossip protocol scales best but has eventual consistency
- Redis pipelining is used for queue operations
- MongoDB operations are optimized with proper indexing

## 📈 Production Metrics

The system exposes comprehensive metrics via Prometheus:

- `job_processing_duration_seconds`: Job processing times
- `election_leader_changes_total`: Leadership changes
- `worker_active_gauge`: Active workers per node
- `queue_depth_gauge`: Jobs in each queue state
- `node_uptime_seconds`: Node uptime tracking

## 🛠️ Troubleshooting

### Common Issues

**Leader Election Not Working**
- Check MongoDB connectivity between nodes
- Verify `NODE_ID` is unique per node
- Ensure election timeouts are appropriate for network latency

**Jobs Not Processing**
- Verify Redis connectivity
- Check if job processors are registered
- Monitor worker pool status via `/api/v1/workers`

**Performance Issues**
- Increase `WORKER_COUNT` for CPU-bound jobs
- Tune retry policies to avoid overwhelming downstream services
- Monitor queue depth and adjust cluster size accordingly

### Debugging Scripts
```bash
# Test MongoDB connectivity
./scripts/test-mongodb.sh

# Monitor cluster status
./scripts/monitor.sh

# Test different algorithms
./scripts/test-algorithms.sh
```

---

**This distributed job processor demonstrates enterprise-grade distributed systems engineering.** It handles the complex challenges of distributed coordination, fault tolerance, and scalability that you'd encounter in production environments. Whether you're processing background jobs, implementing workflow orchestration, or building distributed systems, this codebase provides a solid foundation with battle-tested patterns.

For questions, issues, or contributions, please check the issue tracker or reach out to swe.robertkibet@gmail.com.