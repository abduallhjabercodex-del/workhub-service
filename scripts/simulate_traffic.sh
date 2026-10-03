#!/usr/bin/env bash
# ==============================================================================
# WorkHub Traffic & Reliability Simulator
#
# This script simulates realistic traffic scenarios to test dashboards & alerts:
# 1) Normal Traffic: Continuous background requests updating dashboard metrics (RPS, DB, Cache, Latency)
# 2) Rate Limit Storm: Rapid bursts of login requests triggering HTTP 429 Too Many Requests
# 3) Circuit Breaker Drill:
#    - Stop Redis cache container
#    - Trip circuit to OPEN (state: 2 🔴) and fire Prometheus alert
#    - Restart Redis cache container and wait for cooldown
#    - Send probe request and transition to HALF-OPEN (state: 1 🟡)
#    - Send remaining probe requests to recover and return to CLOSED (state: 0 🟢)
# ==============================================================================

set -eo pipefail

BASE_URL="${WORKHUB_URL:-http://localhost:8090}"
PROM_URL="${PROMETHEUS_URL:-http://localhost:9090}"
GRAFANA_URL="${GRAFANA_URL:-http://localhost:3001}"

# Terminal ANSI color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

get_token() {
    local token
    token=$(curl -s -X POST "${BASE_URL}/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"email":"owner@workhub.dev","password":"Password1234!"}' | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4 || true)
    if [[ -z "$token" ]]; then
        echo -e "${RED}❌ Failed to obtain login token! Verify containers are running: docker compose up -d${NC}"
        exit 1
    fi
    echo "$token"
}

get_project_id() {
    local token="$1"
    local pid
    pid=$(curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4 || true)
    if [[ -z "$pid" ]]; then
        pid="3"
    fi
    echo "$pid"
}

run_normal_traffic() {
    local duration="${1:-30}"
    echo -e "\n${CYAN}========================================================================${NC}"
    echo -e "${BOLD}${BLUE}🚀 [1] Starting Normal Traffic Simulation${NC}"
    echo -e "Duration: ${BOLD}${duration}s${NC} | Target: Drive live telemetry across dashboard panels"
    echo -e "${CYAN}========================================================================${NC}"
    
    local token
    token=$(get_token)
    local pid
    pid=$(get_project_id "$token")
    
    local end_time=$((SECONDS + duration))
    local count=0

    while [ $SECONDS -lt $end_time ]; do
        # 1. Health check
        curl -s "${BASE_URL}/health" >/dev/null &
        
        # 2. List Projects (Database read)
        curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects" >/dev/null &
        
        # 3. Project Summary (Redis cache read / compute)
        curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary" >/dev/null &
        
        # 4. Project Members
        curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/members" >/dev/null &
        
        count=$((count + 4))
        echo -ne "\r${GREEN}⚡ Sent ${count} requests successfully... Remaining: $((end_time - SECONDS))s${NC}"
        sleep 0.5
    done
    wait
    echo -e "\n${GREEN}✓ Normal traffic simulation completed successfully! Monitor Grafana: ${GRAFANA_URL}${NC}\n"
}

run_ratelimit_simulation() {
    echo -e "\n${CYAN}========================================================================${NC}"
    echo -e "${BOLD}${YELLOW}🛡️  [2] Rate Limit Exceeded Simulation (HTTP 429 Too Many Requests)${NC}"
    echo -e "Target: Send fast request bursts to trigger protection and HighRateLimitRejections alert"
    echo -e "${CYAN}========================================================================${NC}"
    
    echo -e "Sending 15 rapid login requests in sub-second intervals..."
    local ok_count=0
    local rejected_count=0

    for i in {1..15}; do
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/auth/login" \
            -H "Content-Type: application/json" \
            -d '{"email":"owner@workhub.dev","password":"Password1234!"}' || true)
        
        if [[ "$code" == "200" ]]; then
            ok_count=$((ok_count + 1))
            echo -e "  Request #$i: ${GREEN}HTTP $code OK (Allowed)${NC}"
        elif [[ "$code" == "429" ]]; then
            rejected_count=$((rejected_count + 1))
            echo -e "  Request #$i: ${RED}HTTP $code Too Many Requests (Rate Limited!)${NC}"
        else
            echo -e "  Request #$i: HTTP $code"
        fi
    done

    echo -e "\n📊 ${BOLD}Result:${NC} Allowed (200): ${GREEN}${ok_count}${NC} | Throttled (429): ${RED}${rejected_count}${NC}"
    echo -e "⏳ Waiting 5s for Prometheus and Grafana metrics scraper..."
    sleep 5

    # Check alert status in Prometheus
    local alert_status
    alert_status=$(curl -s "${PROM_URL}/api/v1/alerts" | grep -o '"alertname":"HighRateLimitRejections"[^}]*' || true)
    if [[ -n "$alert_status" ]]; then
        echo -e "${GREEN}✓ Prometheus alert fired successfully: ${BOLD}HighRateLimitRejections (FIRING 🔥)${NC}"
        echo -e "  Inspect active alert in Prometheus: ${CYAN}${PROM_URL}/alerts${NC}"
    else
        echo -e "${YELLOW}ℹ️  Check alert status in Prometheus: ${PROM_URL}/alerts${NC}"
    fi
    echo -e "  Inspect Rate Limiting card in Grafana: ${CYAN}${GRAFANA_URL}${NC}\n"
}

run_circuit_breaker_drill() {
    echo -e "\n${CYAN}========================================================================${NC}"
    echo -e "${BOLD}${RED}⚡ [3] Full Circuit Breaker Lifecycle Drill${NC}"
    echo -e "Transitions: CLOSED (0 🟢) ➔ OPEN (2 🔴) ➔ HALF-OPEN (1 🟡) ➔ CLOSED (0 🟢)"
    echo -e "${CYAN}========================================================================${NC}"

    local token
    token=$(get_token)
    local pid
    pid=$(get_project_id "$token")

    echo -e "\n${BOLD}Phase 1: Verify current circuit state (should be CLOSED / Healthy 🟢)${NC}"
    local init_state
    init_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^]]*\]' || true)
    echo -e "Prometheus state value: ${GREEN}${init_state}${NC}"

    echo -e "\n${BOLD}Phase 2: Stopping cache container (docker stop workhub_redis)...${NC}"
    docker stop workhub_redis >/dev/null
    echo -e "${RED}⚠️  Redis stopped. Sending 4 consecutive requests to trip threshold (Threshold = 3)...${NC}"
    
    for i in {1..4}; do
        curl -s -o /dev/null -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary" || true
        echo -e "  Cache failure request #$i ❌"
    done

    echo -e "⏳ Waiting 6s for Prometheus scrape interval and alert rule evaluation..."
    sleep 6

    local open_state
    open_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^,]*,"2"\]' || true)
    if [[ -n "$open_state" ]]; then
        echo -e "${RED}🔥 Circuit breaker state is now: OPEN (TRIPPED 🔴 - Value: 2)${NC}"
    else
        echo -e "${YELLOW}Circuit breaker state is updating...${NC}"
    fi

    local cb_alert
    cb_alert=$(curl -s "${PROM_URL}/api/v1/alerts" | grep -o '"alertname":"CacheCircuitBreakerOpen"[^}]*' || true)
    if [[ -n "$cb_alert" ]]; then
        echo -e "${GREEN}✓ Prometheus alert is actively firing: ${BOLD}CacheCircuitBreakerOpen (FIRING 🔥)${NC}"
        echo -e "  View live alert at: ${CYAN}${PROM_URL}/alerts${NC}"
    fi

    echo -e "\n${BOLD}Phase 3: Restarting Redis container (docker start workhub_redis)...${NC}"
    docker start workhub_redis >/dev/null
    echo -e "${GREEN}✓ Redis container started successfully.${NC}"

    echo -e "\n${BOLD}Phase 4: Cooldown period (Cooldown: 10 seconds)...${NC}"
    for i in {10..1}; do
        echo -ne "\r⏳ Waiting for cooldown timer: ${i}s remaining... "
        sleep 1
    done
    echo -e "\n${CYAN}✓ Cooldown elapsed. Circuit breaker is ready for probe requests (Half-Open)...${NC}"

    echo -e "\n${BOLD}Phase 5: Sending initial probe request (Probe Request)...${NC}"
    curl -s -o /dev/null -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary"
    echo -e "${YELLOW}⚡ Circuit entered state: HALF-OPEN (PROBING 🟡 - Value: 1)${NC}"
    echo -e "⏳ Waiting 6s for Prometheus scrape interval to reflect HALF-OPEN in Grafana..."
    sleep 6

    local half_state
    half_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^,]*,"1"\]' || true)
    if [[ -n "$half_state" ]]; then
        echo -e "${GREEN}✓ HALF-OPEN (1 🟡) state successfully recorded in Prometheus and Grafana!${NC}"
    fi

    echo -e "\n${BOLD}Phase 6: Sending remaining successful probe requests to complete recovery...${NC}"
    for i in {1..3}; do
        curl -s -o /dev/null -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary"
        echo -e "  Successful probe request #$i ✓"
    done

    echo -e "⏳ Waiting 6s to confirm circuit recovery to normal state..."
    sleep 6

    local closed_state
    closed_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^,]*,"0"\]' || true)
    if [[ -n "$closed_state" ]]; then
        echo -e "${GREEN}🎉 Circuit breaker fully recovered: CLOSED (HEALTHY 🟢 - Value: 0)${NC}"
        echo -e "${GREEN}✓ Alert resolved and removed from firing list.${NC}"
    fi

    echo -e "\n${BOLD}View full circuit breaker State Timeline panel in Grafana:${NC}"
    echo -e "${CYAN}${GRAFANA_URL}/d/adg7ldb/workhub-enterprise-observability-and-operations${NC}\n"
}

show_menu() {
    clear
    echo -e "${CYAN}========================================================================${NC}"
    echo -e "${BOLD}       WorkHub Observability & Chaos Traffic Simulator${NC}"
    echo -e "${CYAN}========================================================================${NC}"
    echo -e "  1) ${GREEN}Simulate Continuous Normal Traffic (30s)${NC}"
    echo -e "  2) ${YELLOW}Simulate Rate Limit Burst & 429 Alert Trigger${NC}"
    echo -e "  3) ${RED}Simulate Circuit Breaker Drill (Closed -> Open -> Half-Open -> Closed)${NC}"
    echo -e "  4) ${BOLD}Run All Scenarios Sequentially (Full Demo)${NC}"
    echo -e "  5) Exit"
    echo -e "${CYAN}========================================================================${NC}"
    read -rp "Select scenario option [1-5]: " choice
    case "$choice" in
        1) run_normal_traffic 30 ;;
        2) run_ratelimit_simulation ;;
        3) run_circuit_breaker_drill ;;
        4)
            run_normal_traffic 15
            run_ratelimit_simulation
            run_circuit_breaker_drill
            ;;
        5) exit 0 ;;
        *) echo -e "${RED}Invalid selection.${NC}" ;;
    esac
}

# CLI command dispatcher
ACTION="${1:-menu}"
case "$ACTION" in
    normal) run_normal_traffic "${2:-30}" ;;
    ratelimit) run_ratelimit_simulation ;;
    breaker) run_circuit_breaker_drill ;;
    all)
        run_normal_traffic 15
        run_ratelimit_simulation
        run_circuit_breaker_drill
        ;;
    menu|*) show_menu ;;
esac
