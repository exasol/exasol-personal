#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
CLOUD_INIT_ISO="cloud-init.iso"
PID_FILE="qemu.pid"
SHARED_DIR="shared"
VIRTIOFS_SOCKET="virtiofs.sock"
VIRTIOFSD_PID_FILE="virtiofsd.pid"

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
        echo "Run 'task stop-vm' or 'task cleanup-force' to stop it first."
        exit 1
    else
        echo "==> Removing stale PID file..."
        rm -f "$PID_FILE"
    fi
fi

# Create shared directory if it doesn't exist
if [ ! -d "$SHARED_DIR" ]; then
    echo "==> Creating shared directory: $SHARED_DIR"
    mkdir -p "$SHARED_DIR"
fi

# Clean up old files
rm -f "$VIRTIOFSD_PID_FILE"
rm -f "$VIRTIOFS_SOCKET"

echo "==> Starting virtiofsd daemon..."
/usr/libexec/virtiofsd \
    --socket-path="$VIRTIOFS_SOCKET" \
    --shared-dir="$SHARED_DIR" \
    --sandbox none \
    --cache=auto \
    --thread-pool-size=4 &
VIRTIOFSD_PID=$!
echo "$VIRTIOFSD_PID" > "$VIRTIOFSD_PID_FILE"
sleep 1

# Build port forwarding string from manifest
MANIFEST_PORTFWD=$(./scripts/read-manifest-ports.sh || true)
if [ -n "$MANIFEST_PORTFWD" ]; then
    NETDEV_PORTFWD="hostfwd=tcp::2222-:22,$MANIFEST_PORTFWD"
else
    NETDEV_PORTFWD="hostfwd=tcp::2222-:22"
fi

echo "==> Starting Alpine Linux VM in background..."
echo "==> SSH will be available on localhost:2222"
if [ -n "$MANIFEST_PORTFWD" ]; then
    MANIFEST_PORT=$(echo "$MANIFEST_PORTFWD" | grep -oP '(?<=::)\d+(?=-:)' || true)
    if [ -n "$MANIFEST_PORT" ]; then
        echo "==> Container port: localhost:$MANIFEST_PORT"
    fi
fi
echo "==> Shared folder: $SHARED_DIR -> /mnt/host (in VM)"
echo "==> Use: ssh -i vm-key -p 2222 alpine@localhost"
echo ""

qemu-system-aarch64 \
    -machine virt \
    -cpu cortex-a72 \
    -m 2048 \
    -bios /usr/share/qemu-efi-aarch64/QEMU_EFI.fd \
    -drive file="$DISK_IMG",format=raw,if=virtio \
    -drive file="$CLOUD_INIT_ISO",format=raw,if=virtio,readonly=on \
    -netdev user,id=net0,$NETDEV_PORTFWD \
    -device virtio-net-pci,netdev=net0 \
    -chardev socket,id=char0,path="$VIRTIOFS_SOCKET" \
    -device vhost-user-fs-pci,chardev=char0,tag=hostshare \
    -object memory-backend-file,id=mem,size=2G,mem-path=/dev/shm,share=on \
    -numa node,memdev=mem \
    -daemonize \
    -pidfile "$PID_FILE" \
    -display none

echo "==> VM started successfully (PID: $(cat $PID_FILE))"
echo "==> virtiofsd running (PID: $VIRTIOFSD_PID)"
echo "==> Waiting for cloud-init to complete..."

# Wait for cloud-init to complete by polling shared folder
COMPLETION_MARKER="$SHARED_DIR/cloud-init-complete"
MAX_WAIT=300
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    if [ -f "$COMPLETION_MARKER" ]; then
        echo "==> Cloud-init completed successfully!"
        rm -f "$COMPLETION_MARKER"  # Clean up marker file
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

echo "==> VM initialized and ready!"
echo "==> Run 'task connect' to connect via SSH"
echo "==> Run 'task stop-vm' to stop the VM"
