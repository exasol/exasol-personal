#!/usr/bin/env bash
set -euo pipefail

TEST_KEY="test-key"
AUTHORIZED_KEYS="shared/authorized_keys"

# Cleanup function to stop VM and clean up test files
cleanup() {
  local exit_code=$?
  echo ""
  echo "==> Cleaning up..."
  
  # Stop the VM if it's running
  if [ -f "qemu.pid" ]; then
    echo "==> Stopping VM..."
    ./scripts/stop-vm.sh || true
  fi
  
  # Clean up test files
  echo "==> Removing test files..."
  rm -f "$TEST_KEY" "$TEST_KEY.pub"
  rm -f "$AUTHORIZED_KEYS"
  
  exit $exit_code
}

# Set trap to ensure cleanup happens on exit (success or failure)
trap cleanup EXIT INT TERM

echo "==> Testing SSH key import feature"

# Clean up from any previous test runs
rm -f "$TEST_KEY" "$TEST_KEY.pub"
rm -f "$AUTHORIZED_KEYS"

# Generate a new test key
echo "==> Generating test SSH key..."
ssh-keygen -t ed25519 -f "$TEST_KEY" -N "" -C "test-key-for-vm"

# Add the key to authorized_keys in shared folder
echo "==> Adding test key to shared/authorized_keys..."
mkdir -p shared
cat "$TEST_KEY.pub" >> "$AUTHORIZED_KEYS"

# Start the VM
echo "==> Starting VM..."
./scripts/start-vm.sh

# Try to connect with the test key (retry for 5 minutes)
echo "==> Testing SSH connection with test key (will retry for 5 minutes)..."
MAX_WAIT=300  # 5 minutes in seconds
ELAPSED=0
START_TIME=$(date +%s)

while [ $ELAPSED -lt $MAX_WAIT ]; do
    if ssh -i "$TEST_KEY" -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 alpine@localhost "echo 'SSH key import successful!'" 2>/dev/null; then
        echo "==> ✓ Test passed: Successfully connected with imported key after ${ELAPSED} seconds"
        SUCCESS=true
        break
    fi
    
    # Update elapsed time
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))
    
    if [ $ELAPSED -lt $MAX_WAIT ]; then
        echo "==> Connection failed, retrying... (${ELAPSED}s elapsed, ${MAX_WAIT}s timeout)"
        sleep 2
        ELAPSED=$((ELAPSED + 2))
    fi
done

# Report results
if [ "${SUCCESS:-false}" = "true" ]; then
    echo ""
    echo "==> ✓ SSH key import test PASSED"
    exit 0
else
    echo ""
    echo "==> ✗ SSH key import test FAILED"
    exit 1
fi
