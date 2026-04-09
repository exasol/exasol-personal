#!/usr/bin/env bash
set -euo pipefail

PID_FILE="qemu.pid"

if [ ! -f "$PID_FILE" ]; then
    echo "Error: VM is not running (no PID file found)"
    exit 1
fi

PID=$(cat "$PID_FILE")

if ! ps -p "$PID" > /dev/null 2>&1; then
    echo "Error: VM process not found (PID: $PID)"
    echo "==> Removing stale PID file..."
    rm -f "$PID_FILE"
    exit 1
fi

echo "==> Stopping VM (PID: $PID)..."
kill "$PID"

# Wait for process to terminate
for i in {1..10}; do
    if ! ps -p "$PID" > /dev/null 2>&1; then
        echo "==> VM stopped successfully"
        rm -f "$PID_FILE"
        exit 0
    fi
    sleep 0.5
done

# Force kill if still running
if ps -p "$PID" > /dev/null 2>&1; then
    echo "==> Force stopping VM..."
    kill -9 "$PID"
    rm -f "$PID_FILE"
    echo "==> VM stopped"
else
    rm -f "$PID_FILE"
    echo "==> VM stopped successfully"
fi
