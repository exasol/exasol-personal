#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
CLOUD_INIT_ISO="cloud-init.iso"
PID_FILE="qemu.pid"

if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task download-image' first."
    exit 1
fi

if [ ! -f "$CLOUD_INIT_ISO" ]; then
    echo "Error: $CLOUD_INIT_ISO not found. Run 'task create-cloud-init' first."
    exit 1
fi

if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if ps -p "$PID" > /dev/null 2>&1; then
        echo "Error: VM is already running (PID: $PID)"
        echo "Run 'task stop-vm' to stop it first."
        exit 1
    else
        echo "==> Removing stale PID file..."
        rm "$PID_FILE"
    fi
fi

echo "==> Starting Alpine Linux VM in background..."
echo "==> SSH will be available on localhost:2222"
echo "==> Use: ssh -i vm-key -p 2222 alpine@localhost"
echo ""

qemu-system-aarch64 \
    -machine virt \
    -cpu cortex-a72 \
    -m 2048 \
    -bios /usr/share/qemu-efi-aarch64/QEMU_EFI.fd \
    -drive file="$DISK_IMG",format=raw,if=virtio \
    -drive file="$CLOUD_INIT_ISO",format=raw,if=virtio,readonly=on \
    -netdev user,id=net0,hostfwd=tcp::2222-:22 \
    -device virtio-net-pci,netdev=net0 \
    -daemonize \
    -pidfile "$PID_FILE" \
    -display none

echo "==> VM started successfully (PID: $(cat $PID_FILE))"
echo "==> Waiting for cloud-init to complete..."

# Wait for SSH to be available and cloud-init to complete
MAX_WAIT=120
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    if ssh -i vm-key -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=2 alpine@localhost "test -f /tmp/cloud-init-complete" 2>/dev/null; then
        echo "==> Cloud-init completed successfully!"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
    if [ $((ELAPSED % 10)) -eq 0 ]; then
        echo "==> Still waiting... ($ELAPSED seconds elapsed)"
    fi
done

if [ $ELAPSED -ge $MAX_WAIT ]; then
    echo "==> Warning: Timeout waiting for cloud-init to complete"
    echo "==> VM is still running. Run 'task stop-vm' to stop it manually."
    exit 1
fi

echo "==> Stopping VM to save cloud-init configuration..."
./scripts/stop-vm.sh

echo ""
echo "==> Initialization complete!"
echo "==> Disk image is ready with:"
echo "    - Alpine user with password 'alpine'"
echo "    - SSH key configured"
echo "    - SSH server enabled"
echo ""
echo "==> Run 'task start-vm' to start the VM"
