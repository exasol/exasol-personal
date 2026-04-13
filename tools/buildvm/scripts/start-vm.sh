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

# Ensure SSH key is in shared authorized_keys for VM access
if [ -f "vm-key.pub" ]; then
    echo "==> Setting up SSH key for VM access..."
    
    # Create authorized_keys if it doesn't exist
    touch "$SHARED_DIR/authorized_keys"
    chmod 644 "$SHARED_DIR/authorized_keys"
    
    # Add vm-key.pub if not already present
    if ! grep -qF "$(cat vm-key.pub)" "$SHARED_DIR/authorized_keys" 2>/dev/null; then
        cat vm-key.pub >> "$SHARED_DIR/authorized_keys"
    fi
else
    echo "Error: vm-key.pub not found. Run 'task generate-ssh-key' first."
    exit 1
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

# Build port forwarding string
PORTFWD_RULES=""
if [ -f "$VM_CONFIG" ]; then
    # Check if ports array exists in vm-config.json
    HAS_PORTS=$(jq -r 'has("ports")' "$VM_CONFIG" 2>/dev/null || echo "false")
    if [ "$HAS_PORTS" = "true" ]; then
        # Read ports from vm-config.json
        PORT_COUNT=$(jq -r '.ports | length' "$VM_CONFIG" 2>/dev/null || echo "0")
        if [ "$PORT_COUNT" -gt 0 ]; then
            for i in $(seq 0 $((PORT_COUNT - 1))); do
                PROTOCOL=$(jq -r ".ports[$i].protocol" "$VM_CONFIG")
                HOST_PORT=$(jq -r ".ports[$i].host" "$VM_CONFIG")
                VM_PORT=$(jq -r ".ports[$i].vm" "$VM_CONFIG")
                
                if [ -n "$PORTFWD_RULES" ]; then
                    PORTFWD_RULES="$PORTFWD_RULES,hostfwd=$PROTOCOL::$HOST_PORT-:$VM_PORT"
                else
                    PORTFWD_RULES="hostfwd=$PROTOCOL::$HOST_PORT-:$VM_PORT"
                fi
            done
        fi
    fi
fi

# Always include SSH port
if [ -n "$PORTFWD_RULES" ]; then
    NETDEV_PORTFWD="hostfwd=tcp::2222-:22,$PORTFWD_RULES"
else
    NETDEV_PORTFWD="hostfwd=tcp::2222-:22"
fi

echo "==> Starting Alpine Linux VM in background..."
echo "==> SSH will be available on localhost:2222"
if [ -n "$PORTFWD_RULES" ]; then
    # Extract and display forwarded ports
    echo "$PORTFWD_RULES" | tr ',' '\n' | while read -r rule; do
        if [[ "$rule" =~ hostfwd=([^:]+)::([0-9]+)-:([0-9]+) ]]; then
            PROTO="${BASH_REMATCH[1]}"
            HOST="${BASH_REMATCH[2]}"
            VM="${BASH_REMATCH[3]}"
            echo "==> Port forwarding: localhost:$HOST -> VM:$VM ($PROTO)"
        fi
    done
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
