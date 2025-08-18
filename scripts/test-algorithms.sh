#!/bin/bash

set -e

echo "🚀 Distributed Job Processor - Election Algorithm Testing"
echo "========================================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Docker Compose command detection (supports both V1 and V2)
DOCKER_COMPOSE_CMD=""
detect_docker_compose() {
    if command -v docker-compose >/dev/null 2>&1; then
        DOCKER_COMPOSE_CMD="docker-compose"
    elif docker compose version >/dev/null 2>&1; then
        DOCKER_COMPOSE_CMD="docker compose"
    else
        return 1
    fi
    return 0
}

# Function to print colored output
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

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local max_attempts=30
    local attempt=1
    
    print_status "Waiting for service at $url to be ready..."
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url" > /dev/null 2>&1; then
            print_success "Service at $url is ready!"
            return 0
        fi
        
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    print_error "Service at $url failed to start within $((max_attempts * 2)) seconds"
    return 1
}

# Function to test election status
test_election_status() {
    local port=$1
    local node_name=$2
    
    print_status "Testing election status for $node_name (port $port)..."
    
    local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo "$response" | jq -r "\"Node: \(.node_id), Leader: \(.is_leader), Algorithm: \(.current_algorithm)\""
        return 0
    else
        print_warning "Failed to get election status from $node_name"
        return 1
    fi
}

# Function to test cluster consensus
test_cluster_consensus() {
    local ports=("$@")
    local leaders=()
    local algorithm=""
    
    print_status "Testing cluster consensus across ${#ports[@]} nodes..."
    
    for port in "${ports[@]}"; do
        local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
        if [ $? -eq 0 ]; then
            local is_leader=$(echo "$response" | jq -r '.is_leader')
            local node_id=$(echo "$response" | jq -r '.node_id')
            local alg=$(echo "$response" | jq -r '.current_algorithm')
            
            if [ "$algorithm" == "" ]; then
                algorithm=$alg
            fi
            
            if [ "$is_leader" == "true" ]; then
                leaders+=("$node_id")
            fi
        fi
    done
    
    echo "Algorithm: $algorithm"
    echo "Leaders found: ${#leaders[@]}"
    echo "Leader nodes: ${leaders[*]}"
    
    if [ ${#leaders[@]} -eq 1 ]; then
        print_success "✅ Cluster consensus achieved - Single leader: ${leaders[0]}"
        return 0
    elif [ ${#leaders[@]} -eq 0 ]; then
        print_warning "⚠️  No leader found - Election may be in progress"
        return 1
    else
        print_error "❌ Split brain detected - Multiple leaders: ${leaders[*]}"
        return 1
    fi
}

# Function to test job submission
test_job_submission() {
    local port=$1
    
    print_status "Testing job submission on port $port..."
    
    local job_data='{
        "type": "test",
        "priority": 5,
        "payload": {
            "message": "Test job from script",
            "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
        },
        "max_retries": 3
    }'
    
    local response=$(curl -s -X POST "http://localhost:$port/api/v1/jobs" \
        -H "Content-Type: application/json" \
        -d "$job_data" 2>/dev/null)
    
    if [ $? -eq 0 ]; then
        local job_id=$(echo "$response" | jq -r '.id // empty')
        if [ -n "$job_id" ]; then
            print_success "✅ Job created successfully: $job_id"
            return 0
        fi
    fi
    
    print_error "❌ Failed to create job"
    return 1
}

# Function to test failover
test_failover() {
    local compose_file=$1
    local algorithm=$2
    
    print_status "Testing failover scenario for $algorithm algorithm..."
    
    # Get current leader
    local leader_port=""
    local leader_node=""
    
    for port in 8080 8081 8082; do
        local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
        if [ $? -eq 0 ]; then
            local is_leader=$(echo "$response" | jq -r '.is_leader')
            if [ "$is_leader" == "true" ]; then
                leader_port=$port
                leader_node=$(echo "$response" | jq -r '.node_id')
                break
            fi
        fi
    done
    
    if [ -z "$leader_port" ]; then
        print_error "No leader found for failover test"
        return 1
    fi
    
    print_status "Current leader: $leader_node (port $leader_port)"
    
    # Stop the leader container
    local container_name=""
    case $leader_port in
        8080) container_name="job-processor-1" ;;
        8081) container_name="job-processor-2" ;;
        8082) container_name="job-processor-3" ;;
    esac
    
    print_status "Stopping leader container: $container_name"
    $DOCKER_COMPOSE_CMD -f "$compose_file" stop "$container_name"
    
    # Wait for new leader election
    print_status "Waiting for new leader election..."
    sleep 10
    
    # Check for new leader
    local new_leader=""
    for port in 8080 8081 8082; do
        if [ "$port" == "$leader_port" ]; then
            continue # Skip the stopped container
        fi
        
        local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
        if [ $? -eq 0 ]; then
            local is_leader=$(echo "$response" | jq -r '.is_leader')
            if [ "$is_leader" == "true" ]; then
                new_leader=$(echo "$response" | jq -r '.node_id')
                print_success "✅ New leader elected: $new_leader (port $port)"
                break
            fi
        fi
    done
    
    if [ -z "$new_leader" ]; then
        print_error "❌ No new leader elected after failover"
        return 1
    fi
    
    # Restart the stopped container
    print_status "Restarting stopped container: $container_name"
    $DOCKER_COMPOSE_CMD -f "$compose_file" start "$container_name"
    
    return 0
}

# Main testing function
run_algorithm_test() {
    local algorithm=$1
    local compose_file="docker-compose.${algorithm}.yml"
    local ports=()
    
    print_status "Testing $algorithm algorithm..."
    echo "----------------------------------------"
    
    # Set ports based on algorithm
    case $algorithm in
        "bully"|"raft")
            ports=(8080 8081 8082)
            ;;
        "gossip")
            ports=(8080 8081 8082 8083 8084)
            ;;
    esac
    
    # Start the cluster
    print_status "Starting $algorithm cluster..."
    $DOCKER_COMPOSE_CMD -f "$compose_file" up -d
    
    # Wait for services to be ready
    for port in "${ports[@]}"; do
        if ! wait_for_service "http://localhost:$port/health"; then
            print_error "Failed to start cluster for $algorithm"
            $DOCKER_COMPOSE_CMD -f "$compose_file" down
            return 1
        fi
    done
    
    # Wait a bit more for election to settle
    print_status "Waiting for election to settle..."
    sleep 15
    
    # Test election status on all nodes
    echo
    print_status "Election status across all nodes:"
    for port in "${ports[@]}"; do
        test_election_status "$port" "node-$((port-8079))"
    done
    
    echo
    # Test cluster consensus
    if test_cluster_consensus "${ports[@]}"; then
        # Test job submission
        echo
        test_job_submission "${ports[0]}"
        
        # Test failover (only for multi-node setups)
        if [ ${#ports[@]} -gt 1 ]; then
            echo
            test_failover "$compose_file" "$algorithm"
        fi
    fi
    
    # Cleanup
    echo
    print_status "Cleaning up $algorithm cluster..."
    $DOCKER_COMPOSE_CMD -f "$compose_file" down
    
    echo
    print_success "✅ $algorithm algorithm test completed"
    echo
}

# Check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker and try again."
        exit 1
    fi
    
    # Check if docker-compose (V1) or docker compose (V2) is available
    if ! detect_docker_compose; then
        print_error "Docker Compose is not available. Please install docker-compose or use Docker with Compose V2 plugin."
        exit 1
    fi
    print_status "Using Docker Compose command: $DOCKER_COMPOSE_CMD"
    
    # Check if jq is available
    if ! command -v jq >/dev/null 2>&1; then
        print_error "jq is not installed. Please install jq for JSON parsing."
        exit 1
    fi
    
    # Check if curl is available
    if ! command -v curl >/dev/null 2>&1; then
        print_error "curl is not installed. Please install curl."
        exit 1
    fi
    
    print_success "✅ All prerequisites met"
}

# Main script
main() {
    check_prerequisites
    
    echo
    print_status "Available test options:"
    echo "1. Test all algorithms"
    echo "2. Test Bully algorithm only"
    echo "3. Test Raft algorithm only"
    echo "4. Test Gossip algorithm only"
    echo "5. Quick consensus test (existing cluster)"
    echo
    
    if [ $# -eq 0 ]; then
        read -p "Select option (1-5): " choice
    else
        choice=$1
    fi
    
    case $choice in
        1)
            print_status "Testing all election algorithms..."
            run_algorithm_test "bully"
            run_algorithm_test "raft"
            run_algorithm_test "gossip"
            ;;
        2)
            run_algorithm_test "bully"
            ;;
        3)
            run_algorithm_test "raft"
            ;;
        4)
            run_algorithm_test "gossip"
            ;;
        5)
            print_status "Testing consensus on existing cluster..."
            test_cluster_consensus 8080 8081 8082
            ;;
        *)
            print_error "Invalid option. Please select 1-5."
            exit 1
            ;;
    esac
    
    print_success "🎉 All tests completed!"
}

# Run main function with all arguments
main "$@"