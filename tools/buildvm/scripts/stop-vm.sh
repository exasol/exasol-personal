#!/usr/bin/env bash
set -euo pipefail

PID_FILE="qemu.pid"
VIRTIOFSD_PID_FILE="virtiofsd.pid"
VIRTIOFS_SOCKET="virtiofs.sock"

if [ ! -f "$PID_FILE" ]; then
    echo "==> VM is not running (no PID file found)"
    # Still clean up virtiofsd and files
    if [ -f "$VIRTIOFSD_PID_FILE" ]; then
        VIRTIOFSD_PID=$(cat "$VIRTIOFSD_PID_FILE")
        if ps -p "$VIRTIOFSD_PID" > /dev/null 2>&1; then
            echo "==> Stopping virtiofsd (PID: $VIRTIOFSD_PID)..."
            kill "$VIRTIOFSD_PID" 2>/dev/null || true
            sleep 0.5
            if ps -p "$VIRTIOFSD_PID" > /dev/null 2>&1; then
                kill -9 "$VIRTIOFSD_PID" 2>/dev/null || true
            fi
        fi
        rm -f "$VIRTIOFSD_PID_FILE"
    fi
    rm -f "$VIRTIOFS_SOCKET"
    exit 0
fi

PID=$(cat "$PID_FILE")

if ! ps -p "$PID" > /dev/null 2>&1; then
    echo "==> VM process not found (PID: $PID)"
    echo "==> Removing stale PID file..."
    rm -f "$PID_FILE"
    # Still clean up virtiofsd and files
    if [ -f "$VIRTIOFSD_PID_FILE" ]; then
        VIRTIOFSD_PID=$(cat "$VIRTIOFSD_PID_FILE")
        if ps -p "$VIRTIOFSD_PID" > /dev/null 2>&1; then
            echo "==> Stopping virtiofsd (PID: $VIRTIOFSD_PID)..."
            kill "$VIRTIOFSD_PID" 2>/dev/null || true
            sleep 0.5
            if ps -p "$VIRTIOFSD_PID" > /dev/null 2>&1; then
                kill -9 "$VIRTIOFSD_PID" 2>/dev/null || true
            fi
        fi
        rm -f "$VIRTIOFSD_PID_FILE"
    fi
    rm -f "$VIRTIOFS_SOCKET"
    exit 0
fi

echo "==> Stopping VM (PID: $PID)..."
kill "$PID"

# Wait for process to terminate (up to 5 minutes)
for i in {1..600}; do
    if ! ps -p "$PID" > /dev/null 2>&1; then
        echo "==> VM stopped successfully"
        rm -f "$PID_FILE"
        break
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

# Stop virtiofsd daemon if running
if [ -f "$VIRTIOFSD_PID_FILE" ]; then
    VIRTIOFSD_PID=$(cat "$VIRTIOFSD_PID_FILE")
    if ps -p "$VIRTIOFSD_PID" > /dev/null 2>&1; then
        echo "==> Stopping virtiofsd (PID: $VIRTIOFSD_PID)..."
        kill "$VIRTIOFSD_PID" 2>/dev/null || true
        sleep 0.5
        if ps -p "$VIRTIOFSD_PID" > /dev/null 2>&1; then
            kill -9 "$VIRTIOFSD_PID" 2>/dev/null || true
        fi
    fi
    rm -f "$VIRTIOFSD_PID_FILE"
fi

# Clean up socket
rm -f "$VIRTIOFS_SOCKET"