#!/usr/bin/env bash
set -euo pipefail

# Linux VM Startup Script for macOS using vfkit
# Requires: vfkit (install via: brew install vfkit)

# Usage: ./start.sh [cpu_count] [memory_mb] [port_rules] [shared_directory]
# All arguments are optional. Defaults: 2 CPUs, 2048 MB RAM, no port forwarding, no folder sharing
# port_rules format: "protocol:host:vm,protocol:host:vm,..." (e.g., "tcp:8080:8080,tcp:9000:3000")

# Configuration
VM_NAME="Exasol-VM"
DISK_IMG="exasol-vm.img"
SSH_PORT=2222

# Parse command line arguments
CPUS="${1:-2}"
MEMORY_MB="${2:-2048}"
PORT_RULES="${3:-}"
SHARED_DIR="${4:-}"

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

if [[ "$CURRENT_ARCH" != "arm64" && "$CURRENT_ARCH" != "x86_64" ]]; then
    echo "Error: Unsupported architecture: $CURRENT_ARCH"
    exit 1
fi

# vfkit uses Apple Virtualization.framework's built-in EFI — no firmware file needed.
# The variable store is created in the current dir on first boot and persisted.

echo ""
echo "=========================================="
echo "  Starting Linux VM with vfkit"
echo "=========================================="
echo ""
echo "VM Configuration:"
echo "  Name: $VM_NAME"
echo "  CPUs: $CPUS"
echo "  Memory: ${MEMORY_GB}GB (${MEMORY_MB}MB)"
echo "  Disk: $DISK_IMG"
echo "  SSH Port: localhost:$SSH_PORT"
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

# gvproxy provides host-to-guest port forwarding (vfkit does not do this itself).
# The actual guest IP is discovered later from gvproxy's DHCP lease table.
GVPROXY_VFKIT_SOCK="$PWD/gvproxy-vfkit.sock"
GVPROXY_API_SOCK="$PWD/gvproxy-api.sock"

# Persist a random locally-administered MAC so gvproxy sees the same guest across runs.
MAC_FILE="vm-mac.txt"
if [ ! -f "$MAC_FILE" ]; then
    printf '52:54:00:%02x:%02x:%02x\n' \
        $((RANDOM % 256)) $((RANDOM % 256)) $((RANDOM % 256)) > "$MAC_FILE"
fi
VM_MAC=$(cat "$MAC_FILE")

# Build vfkit command arguments
VFKIT_ARGS=(
    --cpus "$CPUS"
    --memory $((MEMORY_GB * 1024))
    --bootloader "efi,variable-store=efi-vars.fd,create"
    --device virtio-blk,path="$DISK_IMG_ABS"
    --device "virtio-net,unixSocketPath=$GVPROXY_VFKIT_SOCK,mac=$VM_MAC"
    --device virtio-rng
    --device virtio-serial,logFilePath=vm-console.log
)

# Add port forwarding rules if provided
if [ -n "$PORT_RULES" ]; then
    IFS=',' read -ra RULES <<< "$PORT_RULES"
    for rule in "${RULES[@]}"; do
        IFS=':' read -r proto host_port vm_port <<< "$rule"
        # vfkit requires separate virtio-net devices for each port forward
        VFKIT_ARGS+=(--device "virtio-net,nat,guestPort=$vm_port,hostPort=$host_port")
        echo "==> Port forwarding: localhost:$host_port -> VM:$vm_port ($proto)"
    done
fi

# Add virtio-fs device if shared directory is provided
# The mountTag must be "hostshare" to match cloud-init configuration
if [ -n "$SHARED_DIR_ABS" ]; then
    VFKIT_ARGS+=(--device "virtio-fs,sharedDir=$SHARED_DIR_ABS,mountTag=hostshare")
fi

# Start gvproxy (provides user-mode NAT + host port forwarding for vfkit)
if [ ! -x "./gvproxy" ]; then
    echo "Error: gvproxy binary missing. Expected at ./gvproxy (bundled with this package)."
    exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq is required to parse vm-config.json. Install with: brew install jq"
    exit 1
fi

rm -f "$GVPROXY_VFKIT_SOCK" "$GVPROXY_API_SOCK" gvproxy.pid
./gvproxy \
    --mtu 1500 \
    --listen "unix://$GVPROXY_API_SOCK" \
    --listen-vfkit "unixgram://$GVPROXY_VFKIT_SOCK" \
    --log-file gvproxy.log \
    --pid-file gvproxy.pid &

for _ in $(seq 1 30); do
    [ -S "$GVPROXY_VFKIT_SOCK" ] && [ -S "$GVPROXY_API_SOCK" ] && break
    sleep 0.2
done
if [ ! -S "$GVPROXY_VFKIT_SOCK" ] || [ ! -S "$GVPROXY_API_SOCK" ]; then
    echo "Error: gvproxy failed to start. Log:"
    cat gvproxy.log 2>/dev/null || true
    exit 1
fi

# Start vfkit in the background
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

# Discover the guest IP from gvproxy's DHCP lease table (matches our VM_MAC).
# gvproxy assigns addresses in order of arrival and reserves some slots, so we
# can't assume a fixed IP — look up the one gvproxy actually handed to us.
GUEST_IP=""
for _ in $(seq 1 30); do
    GUEST_IP=$(curl -s --unix-socket "$GVPROXY_API_SOCK" \
        http:/unix/services/dhcp/leases 2>/dev/null \
        | jq -r --arg mac "$VM_MAC" 'to_entries[] | select(.value == $mac) | .key' \
        | head -1)
    [ -n "$GUEST_IP" ] && break
    sleep 0.5
done
if [ -z "$GUEST_IP" ]; then
    echo "Error: guest did not appear in gvproxy DHCP leases (MAC $VM_MAC)"
    exit 1
fi

# Expose TCP ports from vm-config.json through gvproxy to the discovered guest IP
jq -c '.ports[]? | select(.protocol == "tcp")' vm-config.json 2>/dev/null | while read -r entry; do
    hp=$(printf '%s' "$entry" | jq -r '.host')
    vp=$(printf '%s' "$entry" | jq -r '.vm')
    curl -fsS --unix-socket "$GVPROXY_API_SOCK" \
        http:/unix/services/forwarder/expose \
        -X POST -H 'Content-Type: application/json' \
        -d "{\"local\":\":$hp\",\"remote\":\"$GUEST_IP:$vp\",\"protocol\":\"tcp\"}" \
        > /dev/null
    echo "==> Port forward: localhost:$hp -> $GUEST_IP:$vp"
done

echo "==> VM started successfully!"
echo "==> vfkit PID: $VFKIT_PID"
echo ""
echo "Connection Information:"
echo "  SSH: ssh -i vm-key -p $SSH_PORT exasol@localhost"
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
