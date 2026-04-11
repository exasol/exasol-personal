#!/usr/bin/env bash
set -euo pipefail

echo "==> Force cleanup: Killing all QEMU and virtiofsd processes..."

# Kill all QEMU processes
if pgrep qemu-system > /dev/null 2>&1; then
    echo "==> Killing QEMU processes..."
    pkill -9 qemu-system || true
    sleep 1
else
    echo "==> No QEMU processes found"
fi

# Kill all virtiofsd processes
if pgrep virtiofsd > /dev/null 2>&1; then
    echo "==> Killing virtiofsd processes..."
    pkill -9 virtiofsd || true
    sleep 1
else
    echo "==> No virtiofsd processes found"
fi