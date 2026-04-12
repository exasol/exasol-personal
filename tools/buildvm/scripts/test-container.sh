#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

echo_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup function to stop VM
cleanup() {
  local exit_code=$?
  echo ""
  echo_info "Cleaning up..."
  
  # Stop the VM if it's running
  if [ -f "qemu.pid" ]; then
    echo_info "Stopping VM..."
    ./scripts/stop-vm.sh || true
  fi
  
  exit $exit_code
}

# Set trap to ensure cleanup happens on exit (success or failure)
trap cleanup EXIT INT TERM

echo_info "Testing containerized REST server..."

# Start the VM
echo_info "Starting VM..."
./scripts/start-vm.sh

# Wait for VM to be ready
echo_info "Waiting for VM to initialize..."
sleep 5

# Check if container port is accessible (wait up to 30 seconds)
echo_info "Waiting for container to be ready..."
MAX_WAIT=600
ELAPSED=0
PORT_READY=false

while [ $ELAPSED -lt $MAX_WAIT ]; do
    if curl -s --connect-timeout 1 http://localhost:8080/hello > /dev/null 2>&1; then
        PORT_READY=true
        break
    fi
    sleep 1
    ELAPSED=$((ELAPSED + 1))
done

if [ "$PORT_READY" = "false" ]; then
    echo_error "Container port 8080 not accessible after ${MAX_WAIT} seconds"
    echo_error "Check if container is running: ssh -i vm-key -p 2222 alpine@localhost 'podman ps'"
    echo_error ""
    echo_error "Container load logs:"
    if ls shared/logs/container-load-*.log 1> /dev/null 2>&1; then
        LATEST_LOG=$(ls -t shared/logs/container-load-*.log | head -1)
        echo_error "=== $LATEST_LOG ==="
        cat "$LATEST_LOG" >&2
    else
        echo_error "No container load logs found in shared/logs/"
    fi
    exit 1
fi

echo_info "Container is responding on port 8080"

# Generate unique test data
TEST_DATA="Test data from host at $(date +%s)"
echo_info "Sending POST request with data: '$TEST_DATA'"

# Send POST request and capture response
RESPONSE=$(curl -s -X POST http://localhost:8080/hello -d "$TEST_DATA")

# Verify response is "hello"
if [ "$RESPONSE" != "hello" ]; then
    echo_error "Unexpected response: '$RESPONSE' (expected 'hello')"
    exit 1
fi

echo_info "Received correct response: '$RESPONSE'"

# Wait a moment for file to be written
sleep 1

# Check if data was written to shared folder
DATA_DIR="shared/container-data"
if [ ! -d "$DATA_DIR" ]; then
    echo_error "Data directory not found: $DATA_DIR"
    exit 1
fi

# Find the most recent hello-*.txt file
LATEST_FILE=$(ls -t "$DATA_DIR"/hello-*.txt 2>/dev/null | head -n 1)

if [ -z "$LATEST_FILE" ]; then
    echo_error "No hello-*.txt file found in $DATA_DIR"
    echo_error "Files present: $(ls -la "$DATA_DIR" 2>/dev/null || echo "none")"
    exit 1
fi

echo_info "Found data file: $(basename "$LATEST_FILE")"

# Verify file content matches what we sent
FILE_CONTENT=$(cat "$LATEST_FILE")

if [ "$FILE_CONTENT" != "$TEST_DATA" ]; then
    echo_error "File content mismatch!"
    echo_error "Expected: '$TEST_DATA'"
    echo_error "Got:      '$FILE_CONTENT'"
    exit 1
fi

echo_info "File content matches sent data"

# Print summary
echo ""
echo_info "================================================"
echo_info "Container test PASSED!"
echo_info "================================================"
echo_info "✓ VM is running"
echo_info "✓ Container responding on port 8080"
echo_info "✓ POST /hello returns 'hello'"
echo_info "✓ Data written to shared folder"
echo_info "✓ File content matches sent data"
echo_info "File: $LATEST_FILE"
echo ""
