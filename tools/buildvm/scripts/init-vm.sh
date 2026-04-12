#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
CLOUD_INIT_ISO="cloud-init.iso"
PID_FILE="qemu.pid"
SHARED_DIR="shared"
VIRTIOFS_SOCKET="virtiofs.sock"
VIRTIOFSD_PID_FILE="virtiofsd.pid"
VM_LOG_FILE="vm-init.log"
VM_CONFIG="vm-config.json"

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

# Clear log file from previous runs
> "$VM_LOG_FILE"

# Read VM configuration
if [ -f "$VM_CONFIG" ]; then
    VM_CPUS=$(jq -r '.cpus // 2' "$VM_CONFIG")
    VM_MEMORY=$(jq -r '.memoryMB // 2048' "$VM_CONFIG")
else
    echo "Warning: $VM_CONFIG not found, using defaults"
    VM_CPUS=2
    VM_MEMORY=2048
fi

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
echo "==> VM console output will be logged to: $VM_LOG_FILE"
echo ""

# Detect VM architecture and get QEMU configuration
source ./scripts/get-qemu-args.sh

$QEMU_BIN \
    -machine $QEMU_MACHINE \
    -cpu $QEMU_CPU \
    -m $VM_MEMORY \
    -smp $VM_CPUS \
    -bios "$QEMU_BIOS" \
    -drive file="$DISK_IMG",format=raw,if=virtio \
    -drive file="$CLOUD_INIT_ISO",format=raw,if=virtio,media=cdrom,readonly=on \
    -netdev user,id=net0,$NETDEV_PORTFWD \
    -device virtio-net-pci,netdev=net0 \
    -chardev socket,id=char0,path="$VIRTIOFS_SOCKET" \
    -device vhost-user-fs-pci,chardev=char0,tag=hostshare \
    -object memory-backend-file,id=mem,size=${VM_MEMORY}M,mem-path=/dev/shm,share=on \
    -numa node,memdev=mem \
    -serial file:"$VM_LOG_FILE" \
    -daemonize \
    -pidfile "$PID_FILE" \
    -display none

VM_PID=$(cat $PID_FILE)
echo "==> VM started successfully (PID: $VM_PID)"
echo "==> virtiofsd running (PID: $VIRTIOFSD_PID)"
echo "==> Console output: tail -f $VM_LOG_FILE"
echo "==> Waiting for cloud-init to complete..."
echo ""

# Start tailing the VM log in the background
tail -f "$VM_LOG_FILE" &
TAIL_PID=$!

# Ensure tail process is killed when script exits (for any reason)
trap 'kill $TAIL_PID 2>/dev/null || true; wait $TAIL_PID 2>/dev/null || true' EXIT INT TERM

# Wait for cloud-init to complete by polling shared folder
COMPLETION_MARKER="$SHARED_DIR/cloud-init-complete"
MAX_WAIT=600
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    if [ -f "$COMPLETION_MARKER" ]; then
        # Stop tailing the log
        kill $TAIL_PID 2>/dev/null || true
        wait $TAIL_PID 2>/dev/null || true
        
        echo ""
        echo "==> Cloud-init completed successfully!"
        rm -f "$COMPLETION_MARKER"  # Clean up marker file
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ $ELAPSED -ge $MAX_WAIT ]; then
    # Stop tailing the log
    kill $TAIL_PID 2>/dev/null || true
    wait $TAIL_PID 2>/dev/null || true
    
    echo ""
    echo "==> Warning: Timeout waiting for cloud-init to complete"
    echo "==> VM is still running. Run 'task stop-vm' to stop it manually."
    exit 1
fi

echo "==> Cloud-init finished. VM will now power off..."
echo "==> Waiting for VM to shut down..."

# Wait for VM process to exit
MAX_SHUTDOWN_WAIT=600
SHUTDOWN_ELAPSED=0
while [ $SHUTDOWN_ELAPSED -lt $MAX_SHUTDOWN_WAIT ]; do
    if ! ps -p "$VM_PID" > /dev/null 2>&1; then
        echo "==> VM has powered off successfully"
        rm -f "$PID_FILE"
        break
    fi
    sleep 2
    SHUTDOWN_ELAPSED=$((SHUTDOWN_ELAPSED + 2))
done

if [ $SHUTDOWN_ELAPSED -ge $MAX_SHUTDOWN_WAIT ]; then
    echo "==> Warning: VM did not shut down gracefully within ${MAX_SHUTDOWN_WAIT}s"
    echo "==> Run 'task cleanup-force' if needed"
    exit 1
fi

echo "==> VM initialization complete!"
echo "==> The VM has been configured with SSH keys, packages, and services"
echo "==> Run 'task start-vm' to start the VM"
echo "==> Then 'task connect' to connect via SSH"
