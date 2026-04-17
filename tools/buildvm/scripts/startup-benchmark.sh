#!/usr/bin/env bash
set -euo pipefail

# Ensure VM is stopped on exit (success or failure)
trap './scripts/stop-vm.sh 2>/dev/null || true' EXIT

echo "==> Starting VM startup benchmark..."
echo ""

# Record start time
START_TIME=$(date +%s%3N)  # milliseconds

# Start the VM
./scripts/start-vm.sh

echo ""
echo "==> Waiting for SSH connection..."

# Wait for SSH to be available
MAX_WAIT=180  # 3 minutes for slow emulation
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    if ssh -i vm-key -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=2 exasol@localhost "true" 2>/dev/null; then
        END_TIME=$(date +%s%3N)  # milliseconds
        BOOT_TIME=$(( (END_TIME - START_TIME) / 1000 ))  # Convert to seconds
        BOOT_TIME_MS=$(( (END_TIME - START_TIME) % 1000 ))  # Remainder in ms
        echo ""
        echo "========================================="
        echo "  VM Startup Benchmark Complete"
        echo "========================================="
        echo "  Time to SSH: ${BOOT_TIME}.${BOOT_TIME_MS} seconds"
        echo "========================================="
        echo ""
        exit 0
    fi
    sleep 1
    ELAPSED=$((ELAPSED + 1))
    if [ $((ELAPSED % 10)) -eq 0 ]; then
        printf "."
    fi
done

echo ""
echo "==> Error: Timeout waiting for SSH connection after ${MAX_WAIT} seconds"
exit 1
