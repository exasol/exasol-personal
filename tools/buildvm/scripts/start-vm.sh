#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
PID_FILE="qemu.pid"
SHARED_DIR="shared"
VIRTIOFS_SOCKET="virtiofs.sock"
VIRTIOFSD_PID_FILE="virtiofsd.pid"
VM_LOG_FILE="vm.log"
VM_CONFIG="vm-config.json"

# Check for --attached flag
ATTACHED=false
if [ "${1:-}" = "--attached" ]; then
    ATTACHED=true
fi

if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task init-vm' first."
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
if [ -n " $MANIFEST_PORTFWD" ]; then
    MANIFEST_PORT=$(echo "$MANIFEST_PORTFWD" | grep -oP '(?<=::)\d+(?=-:)' || true)
    if [ -n "$MANIFEST_PORT" ]; then
        echo "==> Container port: localhost:$MANIFEST_PORT"
    fi
fi
echo "==> Shared folder: $SHARED_DIR -> /mnt/host (in VM)"
echo "==> Use: ssh -i vm-key -p 2222 alpine@localhost"
echo "==> Console output: tail -f $VM_LOG_FILE"
echo ""

# Detect VM architecture and get QEMU configuration
source ./scripts/get-qemu-args.sh

if [ "$ATTACHED" = "true" ]; then
    echo "==> Starting VM in attached mode (press Ctrl-A X to exit)..."
    $QEMU_BIN \
        -machine $QEMU_MACHINE \
        -cpu $QEMU_CPU \
        -m $VM_MEMORY \
        -smp $VM_CPUS \
        -bios "$QEMU_BIOS" \
        -drive file="$DISK_IMG",format=raw,if=virtio \
        -netdev user,id=net0,$NETDEV_PORTFWD \
        -device virtio-net-pci,netdev=net0 \
        -chardev socket,id=char0,path="$VIRTIOFS_SOCKET" \
        -device vhost-user-fs-pci,chardev=char0,tag=hostshare \
        -object memory-backend-file,id=mem,size=${VM_MEMORY}M,mem-path=/dev/shm,share=on \
        -numa node,memdev=mem \
        -serial file:"$VM_LOG_FILE" \
        -nographic
    
    # VM has exited
    echo ""
    echo "==> VM stopped"
    rm -f "$PID_FILE"
else
    $QEMU_BIN \
        -machine $QEMU_MACHINE \
        -cpu $QEMU_CPU \
        -m $VM_MEMORY \
        -smp $VM_CPUS \
        -bios "$QEMU_BIOS" \
        -drive file="$DISK_IMG",format=raw,if=virtio \
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

    echo "==> VM started successfully (PID: $(cat $PID_FILE))"
    echo "==> virtiofsd running (PID: $VIRTIOFSD_PID)"
    echo "==> Console output: tail -f $VM_LOG_FILE"
    echo "==> Run 'task stop-vm' to stop the VM"
fi
