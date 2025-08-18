#!/bin/bash

# Real-time monitoring script for distributed job processor
# Usage: ./scripts/monitor.sh [watch|once|metrics|logs]

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
PORTS=(8080 8081 8082 8083 8084)
REFRESH_INTERVAL=3

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

print_header() {
    echo -e "${CYAN}================================================================================================${NC}"
    echo -e "${CYAN} 🔍 DISTRIBUTED JOB PROCESSOR MONITORING DASHBOARD${NC}"
    echo -e "${CYAN}================================================================================================${NC}"
    echo -e "${YELLOW}$(date)${NC}"
    echo
}

get_node_info() {
    local port=$1
    local response=$(curl -s "http://localhost:$port/api/v1/election" 2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$response" ]; then
        echo "$response"
    else
        echo '{"error": "unreachable"}'
    fi
}

get_stats() {
    local port=$1
    local response=$(curl -s "http://localhost:$port/api/v1/stats" 2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$response" ]; then
        echo "$response"
    else
        echo '{"error": "unreachable"}'
    fi
}

get_cluster_info() {
    local port=$1
    local response=$(curl -s "http://localhost:$port/api/v1/cluster" 2>/dev/null)
    
    if [ $? -eq 0 ] && [ -n "$response" ]; then
        echo "$response"
    else
        echo '{"error": "unreachable"}'
    fi
}

show_election_status() {
    echo -e "${BLUE}📊 ELECTION STATUS${NC}"
    echo "==================="
    
    local leader_found=false
    local algorithm=""
    
    for port in "${PORTS[@]}"; do
        local info=$(get_node_info "$port")
        local error=$(echo "$info" | jq -r '.error // empty')
        
        if [ -n "$error" ]; then
            echo -e "${RED}❌ Port $port: $error${NC}"
            continue
        fi
        
        local node_id=$(echo "$info" | jq -r '.node_id')
        local is_leader=$(echo "$info" | jq -r '.is_leader')
        local current_algorithm=$(echo "$info" | jq -r '.current_algorithm')
        local leader_id=$(echo "$info" | jq -r '.leader_id')
        
        if [ -z "$algorithm" ]; then
            algorithm=$current_algorithm
        fi
        
        local status_icon="📱"
        local status_color=$GREEN
        
        if [ "$is_leader" == "true" ]; then
            status_icon="👑"
            status_color=$YELLOW
            leader_found=true
        fi
        
        echo -e "${status_color}$status_icon Port $port: $node_id (Leader: $is_leader)${NC}"
    done
    
    echo
    echo -e "${PURPLE}Algorithm: $algorithm${NC}"
    
    if [ "$leader_found" == "true" ]; then
        echo -e "${GREEN}✅ Leadership status: OK${NC}"
    else
        echo -e "${RED}⚠️  Leadership status: NO LEADER${NC}"
    fi
    
    echo
}

show_job_stats() {
    echo -e "${BLUE}📈 JOB STATISTICS${NC}"
    echo "=================="
    
    local total_pending=0
    local total_running=0
    local total_completed=0
    local total_failed=0
    
    for port in "${PORTS[@]}"; do
        local stats=$(get_stats "$port")
        local error=$(echo "$stats" | jq -r '.error // empty')
        
        if [ -n "$error" ]; then
            continue
        fi
        
        local node_id=$(echo "$stats" | jq -r '.node.id')
        local strategy=$(echo "$stats" | jq -r '.node.strategy')
        local pending=$(echo "$stats" | jq -r '.jobs.pending // 0')
        local running=$(echo "$stats" | jq -r '.jobs.running // 0')
        local completed=$(echo "$stats" | jq -r '.jobs.completed // 0')
        local failed=$(echo "$stats" | jq -r '.jobs.failed // 0')
        
        echo -e "${GREEN}📊 $node_id ($strategy):${NC} Pending: $pending, Running: $running, Completed: $completed, Failed: $failed"
        
        total_pending=$((total_pending + pending))
        total_running=$((total_running + running))
        total_completed=$((total_completed + completed))
        total_failed=$((total_failed + failed))
    done
    
    echo
    echo -e "${CYAN}🔢 TOTALS:${NC} Pending: $total_pending, Running: $total_running, Completed: $total_completed, Failed: $total_failed"
    echo
}

show_queue_stats() {
    echo -e "${BLUE}🔄 QUEUE STATISTICS${NC}"
    echo "==================="
    
    # Get queue stats from first available node
    for port in "${PORTS[@]}"; do
        local stats=$(get_stats "$port")
        local error=$(echo "$stats" | jq -r '.error // empty')
        
        if [ -n "$error" ]; then
            continue
        fi
        
        local queue_pending=$(echo "$stats" | jq -r '.queue.pending // 0')
        local queue_processing=$(echo "$stats" | jq -r '.queue.processing // 0')
        local queue_completed=$(echo "$stats" | jq -r '.queue.completed // 0')
        local queue_failed=$(echo "$stats" | jq -r '.queue.failed // 0')
        local queue_delayed=$(echo "$stats" | jq -r '.queue.delayed // 0')
        
        echo -e "${GREEN}📦 Redis Queue:${NC}"
        echo "  - Pending: $queue_pending"
        echo "  - Processing: $queue_processing"
        echo "  - Completed: $queue_completed"
        echo "  - Failed: $queue_failed"
        echo "  - Delayed: $queue_delayed"
        
        break
    done
    
    echo
}

show_cluster_health() {
    echo -e "${BLUE}🏥 CLUSTER HEALTH${NC}"
    echo "=================="
    
    local healthy_nodes=0
    local total_workers=0
    
    for port in "${PORTS[@]}"; do
        if curl -s "http://localhost:$port/health" >/dev/null 2>&1; then
            healthy_nodes=$((healthy_nodes + 1))
            echo -e "${GREEN}✅ Port $port: Healthy${NC}"
            
            local stats=$(get_stats "$port")
            local error=$(echo "$stats" | jq -r '.error // empty')
            if [ -z "$error" ]; then
                local workers=$(echo "$stats" | jq -r '.node.worker_count // 0')
                total_workers=$((total_workers + workers))
            fi
        else
            echo -e "${RED}❌ Port $port: Unhealthy${NC}"
        fi
    done
    
    echo
    echo -e "${CYAN}Healthy nodes: $healthy_nodes/${#PORTS[@]}${NC}"
    echo -e "${CYAN}Total workers: $total_workers${NC}"
    echo
}

show_recent_metrics() {
    echo -e "${BLUE}📊 PROMETHEUS METRICS (Sample)${NC}"
    echo "==============================="
    
    for port in 9090 9091 9092; do
        local metrics=$(curl -s "http://localhost:$port/metrics" 2>/dev/null | grep -E "(jobs_total|leader_elections|workers_active)" | head -3)
        if [ $? -eq 0 ] && [ -n "$metrics" ]; then
            echo -e "${GREEN}📈 Metrics from port $port:${NC}"
            echo "$metrics"
            echo
            break
        fi
    done
}

show_logs() {
    local node=${1:-"all"}
    
    echo -e "${BLUE}📝 RECENT LOGS${NC}"
    echo "=============="
    
    if detect_docker_compose; then
        if [ "$node" == "all" ]; then
            echo -e "${GREEN}Last 10 lines from all containers:${NC}"
            $DOCKER_COMPOSE_CMD logs --tail=10
        else
            echo -e "${GREEN}Last 20 lines from $node:${NC}"
            $DOCKER_COMPOSE_CMD logs --tail=20 "$node"
        fi
    else
        echo -e "${YELLOW}Docker Compose not available. Check systemd logs or application logs directly.${NC}"
    fi
}

interactive_dashboard() {
    while true; do
        clear
        print_header
        show_election_status
        show_job_stats
        show_queue_stats
        show_cluster_health
        
        echo -e "${CYAN}Press Ctrl+C to exit, or wait ${REFRESH_INTERVAL}s for refresh...${NC}"
        sleep $REFRESH_INTERVAL
    done
}

quick_status() {
    print_header
    show_election_status
    show_job_stats
    show_cluster_health
}

show_help() {
    echo "Distributed Job Processor Monitoring Tool"
    echo "==========================================="
    echo
    echo "Usage: $0 [command]"
    echo
    echo "Commands:"
    echo "  watch    - Interactive dashboard with auto-refresh (default)"
    echo "  once     - Show status once and exit"
    echo "  metrics  - Show Prometheus metrics"
    echo "  logs     - Show recent logs"
    echo "  help     - Show this help message"
    echo
    echo "Examples:"
    echo "  $0                    # Interactive dashboard"
    echo "  $0 once              # Quick status check"
    echo "  $0 metrics           # Show metrics"
    echo "  $0 logs              # Show all logs"
    echo
}

# Main script
main() {
    local command=${1:-watch}
    
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
        "watch")
            trap 'echo; echo "Monitoring stopped."; exit 0' INT
            interactive_dashboard
            ;;
        "once")
            quick_status
            ;;
        "metrics")
            print_header
            show_recent_metrics
            ;;
        "logs")
            print_header
            show_logs "${2:-all}"
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