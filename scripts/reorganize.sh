#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== WorkHub internal/ Reorganization ==="

# 1. Create infrastructure directory
mkdir -p internal/infrastructure

# 2. Move infrastructure packages
for dir in auth bulkhead cache circuitbreaker config featureflag \
           logger messaging metrics retry telemetry version; do
    if [ -d "internal/$dir" ]; then
        echo "→ Moving internal/$dir → internal/infrastructure/$dir"
        mv "internal/$dir" "internal/infrastructure/$dir"
    else
        echo "⚠ internal/$dir not found — skipping"
    fi
done

echo ""
echo "=== 3. Updating Go imports ==="
grep -rlE '"workhub-service/internal/(auth|bulkhead|cache|circuitbreaker|config|featureflag|logger|messaging|metrics|retry|telemetry|version)"' \
     --include='*.go' . | \
xargs sed -i -E 's@workhub-service/internal/(auth|bulkhead|cache|circuitbreaker|config|featureflag|logger|messaging|metrics|retry|telemetry|version)@workhub-service/internal/infrastructure/\1@g'

echo ""
echo "=== 4. Updating helper scripts ==="
if [ -f "scripts/final_qualification.sh" ]; then
    sed -i 's@workhub-service/internal/version@workhub-service/internal/infrastructure/version@g' scripts/final_qualification.sh
    sed -i -E 's@\./internal/(circuitbreaker|bulkhead|auth)/\.\.\.@./internal/infrastructure/\1/...@g' scripts/final_qualification.sh
fi

echo ""
echo "=== 5. Running verification ==="
go build ./...
go test ./...
echo "=== Reorganization completed successfully! ==="