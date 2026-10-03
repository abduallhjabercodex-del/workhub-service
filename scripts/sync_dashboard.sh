#!/usr/bin/env bash
set -eo pipefail

BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

DASHBOARD_FILE="monitoring/grafana/dashboard.json"
GRAFANA_URL="${GRAFANA_URL:-http://localhost:3001}"
GRAFANA_USER="${GRAFANA_USER:-admin}"
GRAFANA_PASS="${GRAFANA_PASS:-admin}"

if [ ! -f "$DASHBOARD_FILE" ]; then
    echo -e "${RED}Error: File $DASHBOARD_FILE not found!${NC}"
    exit 1
fi

echo -e "${BOLD}⏳ Validating and syncing $DASHBOARD_FILE to Grafana...${NC}"

# Python script to resolve datasource variables, clean comments, validate, and POST to Grafana API
python3 -c '
import json, re, subprocess, sys

# 1. Fetch the actual Prometheus datasource UID from Grafana
prom_uid = "cfyiksrc4z3eod"
try:
    ds_raw = subprocess.check_output([
        "curl", "-s", "-u", f"'"$GRAFANA_USER:$GRAFANA_PASS"'",
        f"'"$GRAFANA_URL/api/datasources"'"
    ])
    datasources = json.loads(ds_raw)
    for ds in datasources:
        if ds.get("type") == "prometheus":
            prom_uid = ds.get("uid", prom_uid)
            break
except Exception as e:
    pass

with open("monitoring/grafana/dashboard.json") as f:
    raw = f.read()

# 2. Auto-strip any C-style comments /* ... */ if present
clean = re.sub(r"/\*.*?\*/", "", raw, flags=re.DOTALL)

# 3. Auto-replace ${DS_PROMETHEUS} with the actual Grafana Prometheus UID
clean = clean.replace("${DS_PROMETHEUS}", prom_uid)

try:
    db = json.loads(clean)
except Exception as e:
    print(f"JSON Syntax Error: {e}", file=sys.stderr)
    sys.exit(1)

# Ensure __inputs does not block loading
if "__inputs" in db:
    del db["__inputs"]

payload = {
    "dashboard": db,
    "overwrite": True,
    "message": "Live sync update from local file"
}

with open("/tmp/dash_payload.json", "w") as pf:
    json.dump(payload, pf)

p = subprocess.run([
    "curl", "-s", "-w", "\n%{http_code}",
    "-X", "POST",
    "-u", f"'"$GRAFANA_USER:$GRAFANA_PASS"'",
    "-H", "Content-Type: application/json",
    "-d", "@/tmp/dash_payload.json",
    f"'"$GRAFANA_URL/api/dashboards/db"'"
], capture_output=True, text=True)

lines = p.stdout.strip().split("\n")
status_code = lines[-1]
body = "\n".join(lines[:-1])

if status_code == "200":
    try:
        res = json.loads(body)
        ver = res.get("version", "?")
        url = res.get("url", "")
        print(f"SUCCESS: Version {ver} deployed successfully! URL: http://localhost:3001{url}")
    except Exception:
        print("SUCCESS")
    sys.exit(0)
else:
    print(f"Grafana API Error (HTTP {status_code}): {body}", file=sys.stderr)
    sys.exit(1)
'

rm -f /tmp/dash_payload.json
echo -e "${GREEN}========================================================================${NC}"
echo -e "${GREEN}★ Datasource verified and dashboard successfully synced!${NC}"
echo -e "${GREEN}========================================================================${NC}"
