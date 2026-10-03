#!/usr/bin/env bash
set -eo pipefail

BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

TARGET_FILE="monitoring/grafana/dashboard.json"

echo -e "${BOLD}========================================================================${NC}"
echo -e "${CYAN}★ GRAFANA LIVE DASHBOARD WATCHER ACTIVE${NC}"
echo -e "${BOLD}Watching for changes in: ${TARGET_FILE}${NC}"
echo -e "Any file edit & save (Ctrl + S) will be pushed to Grafana immediately!"
echo -e "${BOLD}========================================================================${NC}"

# Initial sync
./scripts/sync_dashboard.sh

LAST_MOD=$(stat -c %Y "$TARGET_FILE" 2>/dev/null || stat -f %m "$TARGET_FILE" 2>/dev/null)

while true; do
    sleep 1
    CURRENT_MOD=$(stat -c %Y "$TARGET_FILE" 2>/dev/null || stat -f %m "$TARGET_FILE" 2>/dev/null)
    if [ "$CURRENT_MOD" != "$LAST_MOD" ]; then
        echo -e "\n${CYAN}Detected file change in ${TARGET_FILE}! Syncing...${NC}"
        ./scripts/sync_dashboard.sh || true
        LAST_MOD=$CURRENT_MOD
    fi
done
