#!/bin/bash

set -e

echo "🛠️ Local Development Helper for Distributed Job Processor"
echo "========================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Redis is running
check_redis() {
    print_status "Checking Redis status..."
    
    if redis-cli ping >/dev/null 2>&1; then
        print_success "✅ Redis is running"
        return 0
    else
        print_warning "⚠️ Redis is not running"
        return 1
    fi
}

# Start Redis if not running
start_redis() {
    if ! check_redis; then
        print_status "Starting Redis..."
        
        if command -v brew >/dev/null 2>&1; then
            # macOS with Homebrew
            brew services start redis
        elif command -v systemctl >/dev/null 2>&1; then
            # Linux with systemd
            sudo systemctl start redis
        else
            # Try to start Redis directly
            redis-server --daemonize yes
        fi
        
        sleep 2
        
        if check_redis; then
            print_success "✅ Redis started successfully"
        else
            print_error "❌ Failed to start Redis"
            echo "Please start Redis manually:"
            echo "  - macOS: brew services start redis"
            echo "  - Linux: sudo systemctl start redis"
            echo "  - Manual: redis-server"
            exit 1
        fi
    fi
}

# Function to start a single node
start_node() {
    local node_id=$1
    local port=$2
    local metrics_port=$3
    local algorithm=${4:-bully}
    local strategy=${5:-round_robin}
    
    print_status "Starting node $node_id on port $port..."
    
    NODE_ID="$node_id" \
    SERVER_PORT="$port" \
    METRICS_PORT="$metrics_port" \
    ELECTION_ALGORITHM="$algorithm" \
    LOAD_BALANCER_STRATEGY="$strategy" \
    go run cmd/server/main.go &
    
    local pid=$!
    echo "$pid" > "/tmp/job-processor-$node_id.pid"
    
    print_success "✅ Node $node_id started (PID: $pid)"
}

# Function to stop all nodes
stop_nodes() {
    print_status "Stopping all nodes..."
    
    for pid_file in /tmp/job-processor-*.pid; do
        if [ -f "$pid_file" ]; then
            local pid=$(cat "$pid_file")
            local node_id=$(basename "$pid_file" .pid | sed 's/job-processor-//')
            
            if kill -0 "$pid" 2>/dev/null; then
                print_status "Stopping node $node_id (PID: $pid)..."
                kill "$pid"
                rm "$pid_file"
            else
                print_warning "Node $node_id (PID: $pid) was not running"
                rm "$pid_file"
            fi
        fi
    done
    
    print_success "✅ All nodes stopped"
}

# Function to show node status
show_status() {
    print_status "Node status:"
    echo
    
    local found_nodes=false
    
    for pid_file in /tmp/job-processor-*.pid; do
        if [ -f "$pid_file" ]; then
            local pid=$(cat "$pid_file")
            local node_id=$(basename "$pid_file" .pid | sed 's/job-processor-//')
            
            if kill -0 "$pid" 2>/dev/null; then
                echo "✅ $node_id (PID: $pid) - Running"
                found_nodes=true
            else
                echo "❌ $node_id (PID: $pid) - Not running"
                rm "$pid_file"
            fi
        fi
    done
    
    if [ "$found_nodes" = false ]; then
        echo "No nodes currently running"
    fi
    
    echo
    print_status "Quick health check:"
    for port in 8080 8081 8082 8083 8084; do
        if curl -s "http://localhost:$port/health" >/dev/null 2>&1; then
            echo "✅ Port $port - Healthy"
        fi
    done
}

# Function to show election status
show_election_status() {
    print_status "Election status across all running nodes:"
    echo
    
    for port in 8080 8081 8082 8083 8084; do
        local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
        if [ $? -eq 0 ]; then
            local node_id=$(echo "$response" | jq -r '.node_id')
            local is_leader=$(echo "$response" | jq -r '.is_leader')
            local algorithm=$(echo "$response" | jq -r '.current_algorithm')
            local leader_emoji=""
            
            if [ "$is_leader" == "true" ]; then
                leader_emoji="👑"
            fi
            
            echo "$leader_emoji Node: $node_id, Port: $port, Leader: $is_leader, Algorithm: $algorithm"
        fi
    done
}

# Function to monitor nodes in real-time
monitor_nodes() {
    print_status "Starting real-time monitoring (press Ctrl+C to stop)..."
    echo
    
    while true; do
        clear
        echo "🔍 Real-time Node Monitoring"
        echo "==========================="
        echo "$(date)"
        echo
        
        show_election_status
        
        echo
        print_status "System stats:"
        for port in 8080 8081 8082; do
            local response=$(curl -s "http://localhost:$port/api/v1/stats" 2>/dev/null)
            if [ $? -eq 0 ]; then
                local node_id=$(echo "$response" | jq -r '.node.id')
                local strategy=$(echo "$response" | jq -r '.node.strategy')
                echo "📊 $node_id: Strategy=$strategy"
            fi
        done
        
        sleep 5
    done
}

# Function to run tests
run_tests() {
    print_status "Running Go tests..."
    
    echo "Unit tests:"
    go test ./tests/ -v -run "Test.*" | grep -E "(PASS|FAIL|RUN)"
    
    echo
    echo "Benchmarks:"
    go test ./tests/ -bench=. -benchtime=1s
}

# Function to send test jobs
send_test_jobs() {
    local count=${1:-5}
    local port=${2:-8080}
    
    print_status "Sending $count test jobs to port $port..."
    
    for i in $(seq 1 $count); do
        local job_data="{
            \"type\": \"test\",
            \"priority\": $((RANDOM % 10 + 1)),
            \"payload\": {
                \"message\": \"Test job #$i\",
                \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
            },
            \"max_retries\": 3
        }"
        
        local response=$(curl -s -X POST "http://localhost:$port/api/v1/jobs" \
            -H "Content-Type: application/json" \
            -d "$job_data")
        
        if [ $? -eq 0 ]; then
            local job_id=$(echo "$response" | jq -r '.id // "unknown"')
            echo "✅ Job #$i created: $job_id"
        else
            echo "❌ Failed to create job #$i"
        fi
        
        sleep 0.5
    done
    
    print_success "✅ Sent $count test jobs"
}

# Main menu
show_menu() {
    echo
    print_status "Available commands:"
    echo "1.  start-single [algorithm] - Start single node"
    echo "2.  start-cluster [algorithm] - Start 3-node cluster"
    echo "3.  start-large - Start 5-node cluster (gossip)"
    echo "4.  stop - Stop all nodes"
    echo "5.  status - Show node status"
    echo "6.  election - Show election status"
    echo "7.  monitor - Real-time monitoring"
    echo "8.  test - Run Go tests"
    echo "9.  jobs [count] [port] - Send test jobs"
    echo "10. logs [node] - Show logs for node"
    echo
    echo "Algorithms: bully, raft, gossip"
    echo "Load balancing: round_robin, least_loaded, random, priority"
}

# Main script
main() {
    local command=${1:-menu}
    
    case $command in
        "start-single")
            start_redis
            local algorithm=${2:-bully}
            start_node "node-1" 8080 9090 "$algorithm"
            print_success "✅ Single node cluster started with $algorithm algorithm"
            echo "🌐 API: http://localhost:8080"
            echo "📊 Metrics: http://localhost:9090/metrics"
            ;;
            
        "start-cluster")
            start_redis
            local algorithm=${2:-bully}
            start_node "node-1" 8080 9090 "$algorithm"
            start_node "node-2" 8081 9091 "$algorithm"
            start_node "node-3" 8082 9092 "$algorithm"
            print_success "✅ 3-node cluster started with $algorithm algorithm"
            echo "🌐 APIs: http://localhost:8080, http://localhost:8081, http://localhost:8082"
            ;;
            
        "start-large")
            start_redis
            start_node "node-1" 8080 9090 "gossip" "least_loaded"
            start_node "node-2" 8081 9091 "gossip" "least_loaded"
            start_node "node-3" 8082 9092 "gossip" "least_loaded"
            start_node "node-4" 8083 9093 "gossip" "least_loaded"
            start_node "node-5" 8084 9094 "gossip" "least_loaded"
            print_success "✅ 5-node cluster started with gossip algorithm"
            ;;
            
        "stop")
            stop_nodes
            ;;
            
        "status")
            show_status
            ;;
            
        "election")
            show_election_status
            ;;
            
        "monitor")
            monitor_nodes
            ;;
            
        "test")
            run_tests
            ;;
            
        "jobs")
            local count=${2:-5}
            local port=${3:-8080}
            send_test_jobs "$count" "$port"
            ;;
            
        "logs")
            local node=${2:-node-1}
            if [ -f "/tmp/job-processor-$node.pid" ]; then
                local pid=$(cat "/tmp/job-processor-$node.pid")
                print_status "Showing logs for $node (PID: $pid)..."
                # Note: In a real scenario, you'd need to redirect logs to files
                echo "Logs would be shown here (implement log file redirection)"
            else
                print_error "Node $node is not running"
            fi
            ;;
            
        "menu"|*)
            show_menu
            ;;
    esac
}

# Handle Ctrl+C
trap 'echo; print_warning "Interrupted by user"; stop_nodes; exit 0' INT

# Check prerequisites
if ! command -v go >/dev/null 2>&1; then
    print_error "Go is not installed. Please install Go and try again."
    exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
    print_error "jq is not installed. Please install jq for JSON parsing."
    exit 1
fi

# Run main function
main "$@"