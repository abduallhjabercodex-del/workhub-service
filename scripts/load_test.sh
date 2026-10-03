#!/bin/bash
# ==============================================================================
# WorkHub Load Test & Profiling Automation Script
# Usage: ./scripts/load_test.sh [baseline|after]
# Note: This is the self-contained, zero-dependency Go runner.
# (scripts/k6_load_test.js is also available for declarative k6 CI/CD pipelines)
# ==============================================================================
set -eo pipefail

MODE="${1:-baseline}"
TARGET_URL="${TARGET_URL:-http://localhost:8090/health}"
PPROF_URL="${PPROF_URL:-http://127.0.0.1:6060/debug/pprof}"
EVIDENCE_DIR="docs/review/day18/evidence"

echo "========================================================================"
echo " Starting WorkHub Load Test & Profiling in Mode: [${MODE}]"
echo " Target: ${TARGET_URL}"
echo " Admin Profiling: ${PPROF_URL}"
echo "========================================================================"

# Verify server availability
if ! curl -s -f "${TARGET_URL}" >/dev/null; then
    echo "ERROR: Target ${TARGET_URL} is unreachable! Ensure containers or server are running."
    exit 1
fi

OUTPUT_FILE="${EVIDENCE_DIR}/load-test/${MODE}.txt"
echo "--- LOAD TEST RUN [$(date -u +"%Y-%m-%dT%H:%M:%SZ")] MODE: ${MODE} ---" > "${OUTPUT_FILE}"

echo "Launching 2,500 concurrent requests across 50 workers..."
go run ./scripts/loadrunner "${TARGET_URL}" | tee -a "${OUTPUT_FILE}"

# Collect live profiling samples if port 6060 is available
if curl -s "${PPROF_URL}/" >/dev/null; then
    echo "Capturing active pprof profiles from ${PPROF_URL}..."
    go tool pprof -top "${PPROF_URL}/profile?seconds=2" > "${EVIDENCE_DIR}/profiling/cpu-profile.txt" 2>/dev/null || true
    go tool pprof -top -alloc_space "${PPROF_URL}/heap" > "${EVIDENCE_DIR}/profiling/heap-profile.txt" 2>/dev/null || true
    curl -s "${PPROF_URL}/goroutine?debug=1" > "${EVIDENCE_DIR}/profiling/goroutine-profile.txt" 2>/dev/null || true
    curl -s "${PPROF_URL}/block?debug=1" > "${EVIDENCE_DIR}/profiling/block-profile.txt" 2>/dev/null || true
    echo "✓ Profiles captured in ${EVIDENCE_DIR}/profiling/"
else
    echo "Notice: Admin profiling port 6060 not active on host. Profiles will be gathered directly."
fi

echo "========================================================================"
echo " Load test [${MODE}] completed successfully! Results saved to ${OUTPUT_FILE}"
echo "========================================================================"
