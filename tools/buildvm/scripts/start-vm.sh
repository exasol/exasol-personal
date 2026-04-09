#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
PID_FILE="qemu.pid"

if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task init-vm' first."
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
    -netdev user,id=net0,hostfwd=tcp::2222-:22 \
    -device virtio-net-pci,netdev=net0 \
    -daemonize \
    -pidfile "$PID_FILE" \
    -display none

echo "==> VM started successfully (PID: $(cat $PID_FILE))"
echo "==> Run 'task stop-vm' to stop the VM"
