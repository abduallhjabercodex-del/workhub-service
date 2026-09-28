#!/bin/bash
set -eo pipefail

BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BOLD}========================================================================${NC}"
echo -e "${BOLD}           WORKHUB CI/CD PIPELINE (LOCAL & AUTOMATED RUNNER)            ${NC}"
echo -e "${BOLD}========================================================================${NC}"

# -----------------------------------------------------------------------------
# Stage 1: Formatting & Static Analysis
# -----------------------------------------------------------------------------
echo -e "\n${BOLD}[Stage 1/5] Code Formatting & Static Analysis Gate${NC}"
echo "Running gofmt check..."
UNFORMATTED=$(gofmt -l . | grep -v '^vendor/' || true)
if [ -n "$UNFORMATTED" ]; then
    echo -e "${RED}FAILED: Unformatted files detected:${NC}"
    echo "$UNFORMATTED"
    exit 1
fi
echo -e "${GREEN}✓ Code formatting (gofmt): PASS${NC}"

echo "Running go vet static analysis..."
go vet ./...
echo -e "${GREEN}✓ Static analysis (go vet): PASS${NC}"

# -----------------------------------------------------------------------------
# Stage 2: Unit & Integration Tests with Race Detection
# -----------------------------------------------------------------------------
echo -e "\n${BOLD}[Stage 2/5] Tests & Concurrency Race Detection Gate${NC}"
echo "Running all tests with -race..."
go test -v -race ./...
echo -e "${GREEN}✓ Tests & Race checks: PASS${NC}"

# -----------------------------------------------------------------------------
# Stage 3: Vulnerability & Dependency Integrity
# -----------------------------------------------------------------------------
echo -e "\n${BOLD}[Stage 3/5] Security & Dependency Audit Gate${NC}"
echo "Verifying module dependencies integrity..."
go mod verify
echo -e "${GREEN}✓ Dependency checksums & integrity: PASS${NC}"

# -----------------------------------------------------------------------------
# Stage 4: Build & Immutable Artifact Creation
# -----------------------------------------------------------------------------
echo -e "\n${BOLD}[Stage 4/5] Build & Immutable Artifact Creation Gate${NC}"
mkdir -p dist
rm -rf dist/*

echo "Compiling static workhub-server binary..."
CGO_ENABLED=0 go build \
    -tags "netgo,osusergo" \
    -trimpath \
    -ldflags="-s -w -extldflags '-static'" \
    -o dist/workhub-server ./cmd/server

echo "Compiling static workhub-worker binary..."
CGO_ENABLED=0 go build \
    -tags "netgo,osusergo" \
    -trimpath \
    -ldflags="-s -w -extldflags '-static'" \
    -o dist/workhub-worker ./cmd/worker

echo "Generating SHA-256 cryptographic checksums..."
(cd dist && sha256sum workhub-server workhub-worker > checksums.sha256)
cat dist/checksums.sha256

echo "Building Docker container images with commit SHA tag..."
COMMIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "localdev")
docker build -f Dockerfile.api -t workhub-api:${COMMIT_SHA} . >/dev/null
docker build -f Dockerfile.worker -t workhub-worker:${COMMIT_SHA} . >/dev/null

echo "=== IMMUTABLE CONTAINER ARTIFACTS ===" > dist/image-digest.txt
echo "Commit Tag: ${COMMIT_SHA}" >> dist/image-digest.txt
echo "API Image ID: $(docker inspect --format '{{.Id}}' workhub-api:${COMMIT_SHA})" >> dist/image-digest.txt
echo "Worker Image ID: $(docker inspect --format '{{.Id}}' workhub-worker:${COMMIT_SHA})" >> dist/image-digest.txt
echo "Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> dist/image-digest.txt
cat dist/image-digest.txt
echo -e "${GREEN}✓ Immutable artifact creation: PASS${NC}"

# -----------------------------------------------------------------------------
# Stage 5: Release Gate Sign-Off
# -----------------------------------------------------------------------------
echo -e "\n${BOLD}[Stage 5/5] Release Gate Promotion Sign-Off${NC}"
echo "Validating release criteria:"
echo "  [x] Code formatting conforms to standard (gofmt)"
echo "  [x] Static analysis passed without warnings (go vet)"
echo "  [x] 100% tests passed with zero data races (-race)"
echo "  [x] Dependency hashes verified (go mod verify)"
echo "  [x] Cryptographic SHA-256 artifacts generated"
echo "  [x] Immutable container digests recorded"
echo -e "${GREEN}========================================================================${NC}"
echo -e "${GREEN}★ PIPELINE STATUS: 100% PASSED - ARTIFACT APPROVED FOR PRODUCTION DEPLOY${NC}"
echo -e "${GREEN}========================================================================${NC}"
