#!/usr/bin/env bash
set -euo pipefail

# Test that Exasol Nano is running inside the VM and accessible from the host.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
echo_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
echo_error() { echo -e "${RED}[ERROR]${NC} $1"; }

SQL_PORT=8563
UI_PORT=8443

cleanup() {
  local exit_code=$?
  echo ""
  echo_info "Cleaning up..."
  if [ -f "qemu.pid" ]; then
    echo_info "Stopping VM..."
    ./scripts/stop-vm.sh || true
  fi
  exit $exit_code
}

trap cleanup EXIT INT TERM

echo_info "Testing Exasol Nano in VM..."

# Start the VM
echo_info "Starting VM..."
./scripts/start-vm.sh

echo_info "Waiting for VM to boot..."
sleep 10

# Wait for SQL port to be reachable
echo_info "Waiting for Exasol Nano SQL port ($SQL_PORT) to be ready..."
MAX_WAIT=300
ELAPSED=0
SQL_READY=false

while [ $ELAPSED -lt $MAX_WAIT ]; do
    if nc -z -w 1 localhost $SQL_PORT 2>/dev/null; then
        SQL_READY=true
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
    if [ $((ELAPSED % 30)) -eq 0 ]; then
        echo_info "  Still waiting... (${ELAPSED}s / ${MAX_WAIT}s)"
    fi
done

if [ "$SQL_READY" = "false" ]; then
    echo_error "SQL port $SQL_PORT not accessible after ${MAX_WAIT} seconds"
    echo_error ""
    echo_error "Checking VM container status..."
    ssh -i vm-key -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        exasol@localhost 'sudo podman ps -a' 2>/dev/null || echo_error "Could not SSH into VM"
    echo_error ""
    echo_error "Container load logs:"
    if ls shared/logs/container-load-*.log 1> /dev/null 2>&1; then
        LATEST_LOG=$(ls -t shared/logs/container-load-*.log | head -1)
        echo_error "=== $LATEST_LOG ==="
        cat "$LATEST_LOG" >&2
    fi
    if ls shared/logs/container-runtime-*.log 1> /dev/null 2>&1; then
        LATEST_RUNTIME_LOG=$(ls -t shared/logs/container-runtime-*.log | head -1)
        echo_error "=== $LATEST_RUNTIME_LOG ==="
        cat "$LATEST_RUNTIME_LOG" >&2
    fi
    exit 1
fi

echo_info "SQL port $SQL_PORT is reachable (after ${ELAPSED}s)"

# Check UI port
echo_info "Checking UI port ($UI_PORT)..."
UI_READY=false
UI_WAIT=60
UI_ELAPSED=0
while [ $UI_ELAPSED -lt $UI_WAIT ]; do
    if nc -z -w 1 localhost $UI_PORT 2>/dev/null; then
        UI_READY=true
        break
    fi
    sleep 2
    UI_ELAPSED=$((UI_ELAPSED + 2))
done

if [ "$UI_READY" = "true" ]; then
    echo_info "UI port $UI_PORT is reachable"
else
    echo_warn "UI port $UI_PORT not reachable after ${UI_WAIT}s (may still be starting)"
fi

# Show container status inside VM
echo_info "Container status inside VM:"
ssh -i vm-key -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    exasol@localhost 'sudo podman ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"' 2>/dev/null || true

echo ""
echo_info "================================================"
echo_info "Exasol Nano VM test PASSED!"
echo_info "================================================"
echo_info "  SQL port: localhost:$SQL_PORT"
if [ "$UI_READY" = "true" ]; then
    echo_info "  UI port:  localhost:$UI_PORT"
fi
echo ""
