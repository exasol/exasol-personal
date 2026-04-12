#!/usr/bin/env bash
set -euo pipefail

# Alpine Linux VM Startup Script for macOS using vfkit
# Requires: vfkit (install via: brew install vfkit)

# Usage: ./start.sh [cpu_count] [memory_mb] [shared_directory]
# All arguments are optional. Defaults: 2 CPUs, 2048 MB RAM, no folder sharing

# Configuration
VM_NAME="Alpine-VM"
DISK_IMG="alpine-vm.img"
SSH_PORT=2222

# Parse command line arguments
CPUS="${1:-2}"
MEMORY_MB="${2:-2048}"
SHARED_DIR="${3:-}"

# Convert memory from MB to GB for vfkit
MEMORY_GB=$((MEMORY_MB / 1024))

# Check if vfkit is installed
if ! command -v vfkit &> /dev/null; then
    echo "Error: vfkit is not installed"
    echo "Install it with: brew install vfkit"
    exit 1
fi

# Check if disk image exists
if [ ! -f "$DISK_IMG" ]; then
    echo "Error: Disk image not found: $DISK_IMG"
    exit 1
fi

# Resolve disk image to absolute path
DISK_IMG_ABS="$(cd "$(dirname "$DISK_IMG")" && pwd)/$(basename "$DISK_IMG")"

# Validate and resolve shared directory if provided
SHARED_DIR_ABS=""
if [ -n "$SHARED_DIR" ]; then
    if [ ! -d "$SHARED_DIR" ]; then
        echo "Error: Shared directory not found: $SHARED_DIR"
        exit 1
    fi
    SHARED_DIR_ABS="$(cd "$SHARED_DIR" && pwd)"
    echo "==> Shared directory: $SHARED_DIR_ABS"
fi

# Detect architecture
CURRENT_ARCH=$(uname -m)
echo "==> Detected host architecture: $CURRENT_ARCH"

# Configure based on architecture
if [[ "$CURRENT_ARCH" == "arm64" ]]; then
    BOOTLOADER="/opt/homebrew/share/vfkit/edk2-aarch64-code.fd"
    echo "==> Using ARM64 configuration"
elif [[ "$CURRENT_ARCH" == "x86_64" ]]; then
    BOOTLOADER="/usr/local/share/vfkit/edk2-x86_64-code.fd"
    echo "==> Using x86_64 configuration"
else
    echo "Error: Unsupported architecture: $CURRENT_ARCH"
    exit 1
fi

# Check if bootloader exists
if [ ! -f "$BOOTLOADER" ]; then
    echo "Warning: UEFI bootloader not found at expected location: $BOOTLOADER"
    echo "Attempting to locate alternative bootloader..."
    
    # Try to find bootloader in common locations
    POSSIBLE_LOCATIONS=(
        "/opt/homebrew/Cellar/vfkit/*/share/vfkit/edk2-${CURRENT_ARCH/arm64/aarch64}-code.fd"
        "/usr/local/Cellar/vfkit/*/share/vfkit/edk2-${CURRENT_ARCH/arm64/aarch64}-code.fd"
        "/opt/homebrew/share/vfkit/edk2-${CURRENT_ARCH/arm64/aarch64}-code.fd"
    )
    
    BOOTLOADER=""
    for pattern in "${POSSIBLE_LOCATIONS[@]}"; do
        # Use glob expansion
        for file in $pattern; do
            if [ -f "$file" ]; then
                BOOTLOADER="$file"
                echo "==> Found bootloader: $BOOTLOADER"
                break 2
            fi
        done
    done
    
    if [ -z "$BOOTLOADER" ]; then
        echo "Error: Could not locate UEFI bootloader for $CURRENT_ARCH"
        echo "Please ensure vfkit is properly installed with: brew reinstall vfkit"
        exit 1
    fi
fi

echo ""
echo "=========================================="
echo "  Starting Alpine Linux VM with vfkit"
echo "=========================================="
echo ""
echo "VM Configuration:"
echo "  Name: $VM_NAME"
echo "  CPUs: $CPUS"
echo "  Memory: ${MEMORY_GB}GB (${MEMORY_MB}MB)"
echo "  Disk: $DISK_IMG"
echo "  SSH Port: localhost:$SSH_PORT"
echo "  Bootloader: $BOOTLOADER"
if [ -n "$SHARED_DIR_ABS" ]; then
    echo "  Shared Folder: $SHARED_DIR_ABS -> /mnt/host (in VM)"
else
    echo "  Shared Folder: None (provide as 3rd argument to enable)"
fi
echo ""

# Check if VM is already running by checking for vfkit process
if pgrep -f "vfkit.*$DISK_IMG" > /dev/null; then
    echo "Error: VM appears to be already running (vfkit process found)"
    echo "To stop it, run: killall vfkit"
    exit 1
fi

echo "==> Starting VM..."
echo "==> VM will run in the background. Monitor with: ps aux | grep vfkit"
echo ""

# Build vfkit command arguments
VFKIT_ARGS=(
    --cpus "$CPUS"
    --memory $((MEMORY_GB * 1024))
    --bootloader "$BOOTLOADER"
    --device virtio-blk,path="$DISK_IMG_ABS"
    --device virtio-net,nat,guestPort=22,hostPort=$SSH_PORT
    --device virtio-rng
    --device virtio-serial,logFilePath=vm-console.log
)

# Add virtio-fs device if shared directory is provided
# The mountTag must be "hostshare" to match cloud-init configuration
if [ -n "$SHARED_DIR_ABS" ]; then
    VFKIT_ARGS+=(--device "virtio-fs,sharedDir=$SHARED_DIR_ABS,mountTag=hostshare")
fi

# Start vfkit in the background
# Note: vfkit uses different syntax than QEMU
# --cpus, --memory (in MiB), --device for virtio devices
vfkit "${VFKIT_ARGS[@]}" > vfkit.log 2>&1 &

VFKIT_PID=$!
echo "$VFKIT_PID" > vfkit.pid

# Wait a moment for VM to start
sleep 2

# Check if vfkit is still running
if ! kill -0 "$VFKIT_PID" 2>/dev/null; then
    echo "Error: vfkit failed to start"
    echo "Check vfkit.log for details:"
    cat vfkit.log
    exit 1
fi

echo "==> VM started successfully!"
echo "==> vfkit PID: $VFKIT_PID"
echo ""
echo "Connection Information:"
echo "  SSH: ssh -i vm-key -p $SSH_PORT alpine@localhost"
echo "  Console log: tail -f vm-console.log"
echo "  vfkit log: tail -f vfkit.log"
if [ -n "$SHARED_DIR_ABS" ]; then
    echo ""
    echo "Shared Folder:"
    echo "  Host: $SHARED_DIR_ABS"
    echo "  VM:   /mnt/host"
    echo "  Files in the host directory will be accessible inside the VM at /mnt/host"
fi
echo ""
echo "Management Commands:"
echo "  Stop VM: kill $VFKIT_PID"
echo "  Or:      killall vfkit"
echo "  Check status: ps aux | grep vfkit"
echo ""
echo "Note: The VM is running in the background"
echo "Wait 20-30 seconds for the VM to fully boot before connecting"
echo ""
