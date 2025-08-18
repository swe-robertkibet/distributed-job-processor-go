#!/bin/bash

# Debug and troubleshooting script for distributed job processor
# Usage: ./scripts/debug.sh [command] [options]

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

PORTS=(8080 8081 8082 8083 8084)

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

print_section() {
    echo -e "${CYAN}$1${NC}"
    echo "$(printf '=%.0s' {1..60})"
}

# Health check for all services
health_check() {
    print_section "🏥 HEALTH CHECK"
    
    echo "Checking core services..."
    echo
    
    # Check Redis
    echo -n "Redis (6379): "
    if redis-cli ping >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Connected${NC}"
    else
        echo -e "${RED}❌ Not reachable${NC}"
    fi
    
    # Check MongoDB
    echo -n "MongoDB: "
    if curl -s "mongodb://localhost:27017" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ Connected${NC}"
    else
        echo -e "${YELLOW}⚠️  Local MongoDB not detected (using Atlas)${NC}"
    fi
    
    echo
    echo "Checking job processor nodes..."
    echo
    
    for port in "${PORTS[@]}"; do
        echo -n "Node on port $port: "
        local health=$(curl -s "http://localhost:$port/health" 2>/dev/null)
        
        if [ $? -eq 0 ]; then
            local status=$(echo "$health" | jq -r '.status // "unknown"')
            local node_id=$(echo "$health" | jq -r '.node_id // "unknown"')
            echo -e "${GREEN}✅ $status ($node_id)${NC}"
        else
            echo -e "${RED}❌ Not reachable${NC}"
        fi
    done
    
    echo
}

# Network connectivity test
network_test() {
    print_section "🌐 NETWORK CONNECTIVITY"
    
    echo "Testing internal connectivity..."
    echo
    
    # Test Redis connectivity from different contexts
    echo "Redis connectivity:"
    echo -n "  Direct connection: "
    if redis-cli ping >/dev/null 2>&1; then
        echo -e "${GREEN}✅ OK${NC}"
    else
        echo -e "${RED}❌ Failed${NC}"
    fi
    
    echo -n "  Docker network: "
    if docker run --rm --network distributed-job-processor-go_default redis:alpine redis-cli -h redis ping >/dev/null 2>&1; then
        echo -e "${GREEN}✅ OK${NC}"
    else
        echo -e "${RED}❌ Failed${NC}"
    fi
    
    echo
    echo "Node-to-node connectivity:"
    
    for source_port in 8080 8081 8082; do
        echo "From port $source_port:"
        for target_port in 8080 8081 8082; do
            if [ "$source_port" != "$target_port" ]; then
                echo -n "  -> $target_port: "
                # Simulate connection test (in real scenario, you'd test actual node communication)
                if curl -s "http://localhost:$target_port/health" >/dev/null 2>&1; then
                    echo -e "${GREEN}✅ Reachable${NC}"
                else
                    echo -e "${RED}❌ Unreachable${NC}"
                fi
            fi
        done
    done
    
    echo
}

# Election debugging
election_debug() {
    print_section "🗳️ ELECTION DEBUGGING"
    
    echo "Current election state across all nodes:"
    echo
    
    local leaders=()
    local algorithms=()
    local nodes_data=()
    
    for port in "${PORTS[@]}"; do
        local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
        
        if [ $? -eq 0 ] && [ -n "$response" ]; then
            local node_id=$(echo "$response" | jq -r '.node_id')
            local is_leader=$(echo "$response" | jq -r '.is_leader')
            local algorithm=$(echo "$response" | jq -r '.current_algorithm')
            local leader_id=$(echo "$response" | jq -r '.leader_id')
            
            algorithms+=("$algorithm")
            nodes_data+=("$port:$node_id:$is_leader:$leader_id")
            
            if [ "$is_leader" == "true" ]; then
                leaders+=("$node_id")
            fi
            
            echo "📊 Port $port ($node_id):"
            echo "   Algorithm: $algorithm"
            echo "   Is Leader: $is_leader"
            echo "   Knows Leader: $leader_id"
            echo
        else
            echo "❌ Port $port: Unreachable"
            echo
        fi
    done
    
    # Analysis
    echo "🔍 Analysis:"
    
    # Check algorithm consistency
    local unique_algorithms=($(printf '%s\n' "${algorithms[@]}" | sort -u))
    if [ ${#unique_algorithms[@]} -eq 1 ]; then
        echo -e "${GREEN}✅ Algorithm consistency: All nodes using ${unique_algorithms[0]}${NC}"
    else
        echo -e "${RED}❌ Algorithm mismatch: ${unique_algorithms[*]}${NC}"
    fi
    
    # Check leader count
    if [ ${#leaders[@]} -eq 0 ]; then
        echo -e "${RED}❌ No leader elected${NC}"
    elif [ ${#leaders[@]} -eq 1 ]; then
        echo -e "${GREEN}✅ Single leader: ${leaders[0]}${NC}"
    else
        echo -e "${RED}❌ Split brain detected: ${leaders[*]}${NC}"
    fi
    
    # Check leader consensus
    local known_leaders=()
    for data in "${nodes_data[@]}"; do
        IFS=':' read -ra ADDR <<< "$data"
        local leader_id="${ADDR[3]}"
        if [ "$leader_id" != "null" ] && [ "$leader_id" != "" ]; then
            known_leaders+=("$leader_id")
        fi
    done
    
    local unique_known_leaders=($(printf '%s\n' "${known_leaders[@]}" | sort -u))
    if [ ${#unique_known_leaders[@]} -le 1 ]; then
        echo -e "${GREEN}✅ Leader consensus: Good${NC}"
    else
        echo -e "${RED}❌ Leader consensus issue: ${unique_known_leaders[*]}${NC}"
    fi
    
    echo
}

# Performance analysis
performance_analysis() {
    print_section "⚡ PERFORMANCE ANALYSIS"
    
    echo "Resource usage and performance metrics:"
    echo
    
    # Memory and CPU usage (if available)
    if command -v docker >/dev/null 2>&1; then
        echo "Docker container stats:"
        docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}" | grep -E "(CONTAINER|job-processor)"
        echo
    fi
    
    # Job processing rates
    echo "Job processing statistics:"
    for port in "${PORTS[@]}"; do
        local stats=$(curl -s "http://localhost:$port/api/v1/stats" 2>/dev/null)
        
        if [ $? -eq 0 ] && [ -n "$stats" ]; then
            local node_id=$(echo "$stats" | jq -r '.node.id')
            local completed=$(echo "$stats" | jq -r '.jobs.completed // 0')
            local failed=$(echo "$stats" | jq -r '.jobs.failed // 0')
            local processing=$(echo "$stats" | jq -r '.jobs.running // 0')
            
            if [ "$completed" -gt 0 ] || [ "$failed" -gt 0 ]; then
                local success_rate=$(echo "scale=2; $completed / ($completed + $failed) * 100" | bc -l 2>/dev/null || echo "N/A")
                echo "📈 $node_id: Completed: $completed, Failed: $failed, Success Rate: ${success_rate}%, Active: $processing"
            else
                echo "📈 $node_id: No jobs processed yet"
            fi
        fi
    done
    
    echo
    
    # Response time test
    echo "API response time test:"
    for port in "${PORTS[@]}"; do
        echo -n "Port $port: "
        local start_time=$(date +%s%3N)
        curl -s "http://localhost:$port/health" >/dev/null 2>&1
        local end_time=$(date +%s%3N)
        local response_time=$((end_time - start_time))
        
        if [ $? -eq 0 ]; then
            if [ $response_time -lt 100 ]; then
                echo -e "${GREEN}${response_time}ms ✅${NC}"
            elif [ $response_time -lt 500 ]; then
                echo -e "${YELLOW}${response_time}ms ⚠️${NC}"
            else
                echo -e "${RED}${response_time}ms ❌${NC}"
            fi
        else
            echo -e "${RED}Timeout ❌${NC}"
        fi
    done
    
    echo
}

# Log analysis
log_analysis() {
    print_section "📝 LOG ANALYSIS"
    
    if detect_docker_compose; then
        echo "Recent error patterns in logs:"
        echo
        
        # Look for common error patterns
        local error_patterns=("ERROR" "FATAL" "panic" "failed" "timeout" "connection refused")
        
        for pattern in "${error_patterns[@]}"; do
            local count=$($DOCKER_COMPOSE_CMD logs --tail=100 2>/dev/null | grep -i "$pattern" | wc -l)
            if [ "$count" -gt 0 ]; then
                echo -e "${RED}$pattern: $count occurrences${NC}"
            fi
        done
        
        echo
        echo "Recent significant events:"
        $DOCKER_COMPOSE_CMD logs --tail=50 2>/dev/null | grep -E "(leader|election|failed|error)" | tail -10
        
    else
        echo "Docker Compose not available. Please check logs manually."
    fi
    
    echo
}

# Configuration validation
config_validation() {
    print_section "⚙️ CONFIGURATION VALIDATION"
    
    echo "Checking configuration consistency:"
    echo
    
    # Check environment variables
    local env_file=".env"
    if [ -f "$env_file" ]; then
        echo "Environment file found: $env_file"
        
        # Check critical variables
        local critical_vars=("MONGODB_URI" "REDIS_ADDR" "ELECTION_ALGORITHM")
        for var in "${critical_vars[@]}"; do
            local value=$(grep "^$var=" "$env_file" | cut -d'=' -f2)
            if [ -n "$value" ]; then
                echo -e "${GREEN}✅ $var: Set${NC}"
            else
                echo -e "${RED}❌ $var: Missing${NC}"
            fi
        done
    else
        echo -e "${YELLOW}⚠️  No .env file found${NC}"
    fi
    
    echo
    
    # Check algorithm configuration across nodes
    echo "Algorithm configuration across nodes:"
    local algorithms=()
    
    for port in "${PORTS[@]}"; do
        local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
        
        if [ $? -eq 0 ] && [ -n "$response" ]; then
            local algorithm=$(echo "$response" | jq -r '.current_algorithm')
            local node_id=$(echo "$response" | jq -r '.node_id')
            algorithms+=("$algorithm")
            echo "  $node_id: $algorithm"
        fi
    done
    
    # Check consistency
    local unique_algorithms=($(printf '%s\n' "${algorithms[@]}" | sort -u))
    if [ ${#unique_algorithms[@]} -eq 1 ]; then
        echo -e "${GREEN}✅ All nodes using same algorithm${NC}"
    else
        echo -e "${RED}❌ Algorithm mismatch detected${NC}"
    fi
    
    echo
}

# Troubleshooting suggestions
troubleshooting_suggestions() {
    print_section "🔧 TROUBLESHOOTING SUGGESTIONS"
    
    echo "Common issues and solutions:"
    echo
    
    # Detect docker compose command first
    detect_docker_compose >/dev/null 2>&1 || DOCKER_COMPOSE_CMD="docker compose"
    
    echo "1. No nodes responding:"
    echo "   - Check if Docker containers are running: $DOCKER_COMPOSE_CMD ps"
    echo "   - Check if Redis is running: redis-cli ping"
    echo "   - Verify ports are not in use: netstat -tulpn | grep :8080"
    echo
    
    echo "2. No leader elected:"
    echo "   - Wait for election timeout to complete"
    echo "   - Check network connectivity between nodes"
    echo "   - Restart all nodes: $DOCKER_COMPOSE_CMD restart"
    echo
    
    echo "3. Split brain scenario:"
    echo "   - Stop all nodes: $DOCKER_COMPOSE_CMD down"
    echo "   - Clear any stale data: docker volume prune"
    echo "   - Restart cluster: $DOCKER_COMPOSE_CMD up -d"
    echo
    
    echo "4. Jobs not processing:"
    echo "   - Check if workers are registered: curl http://localhost:8080/api/v1/workers"
    echo "   - Verify Redis connectivity: redis-cli ping"
    echo "   - Check job processor registration in logs"
    echo
    
    echo "5. High latency:"
    echo "   - Check system resources: docker stats"
    echo "   - Monitor network latency between nodes"
    echo "   - Consider adjusting timeout values"
    echo
}

# Full diagnostic report
full_diagnostic() {
    echo -e "${CYAN}🔍 FULL DIAGNOSTIC REPORT${NC}"
    echo "$(printf '=%.0s' {1..80})"
    echo "Generated at: $(date)"
    echo
    
    health_check
    echo
    network_test
    echo
    election_debug
    echo
    performance_analysis
    echo
    config_validation
    echo
    log_analysis
    echo
    troubleshooting_suggestions
}

# Show help
show_help() {
    echo "Distributed Job Processor Debug Tool"
    echo "====================================="
    echo
    echo "Usage: $0 [command]"
    echo
    echo "Commands:"
    echo "  health       - Quick health check of all services"
    echo "  network      - Test network connectivity"
    echo "  election     - Debug election process and leadership"
    echo "  performance  - Analyze performance metrics"
    echo "  config       - Validate configuration"
    echo "  logs         - Analyze recent logs for issues"
    echo "  suggestions  - Show troubleshooting suggestions"
    echo "  full         - Run full diagnostic report (default)"
    echo "  help         - Show this help message"
    echo
    echo "Examples:"
    echo "  $0                # Full diagnostic report"
    echo "  $0 health        # Quick health check"
    echo "  $0 election      # Debug election issues"
    echo
}

# Main function
main() {
    local command=${1:-full}
    
    # Check prerequisites
    if ! command -v curl >/dev/null 2>&1; then
        echo -e "${RED}Error: curl is required but not installed.${NC}"
        exit 1
    fi
    
    if ! command -v jq >/dev/null 2>&1; then
        echo -e "${RED}Error: jq is required but not installed.${NC}"
        exit 1
    fi
    
    case $command in
        "health")
            health_check
            ;;
        "network")
            network_test
            ;;
        "election")
            election_debug
            ;;
        "performance")
            performance_analysis
            ;;
        "config")
            config_validation
            ;;
        "logs")
            log_analysis
            ;;
        "suggestions")
            troubleshooting_suggestions
            ;;
        "full")
            full_diagnostic
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            echo -e "${RED}Unknown command: $command${NC}"
            echo
            show_help
            exit 1
            ;;
    esac
}

# Run main function
main "$@"