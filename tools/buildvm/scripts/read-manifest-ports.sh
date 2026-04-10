#!/usr/bin/env bash
# Read container manifest and output QEMU port forwarding arguments

MANIFEST_FILE="shared/container-manifest.json"

# Exit silently if manifest doesn't exist (backward compatibility)
if [ ! -f "$MANIFEST_FILE" ]; then
    exit 0
fi

# Check if jq is installed (should be on host for this script)
if ! command -v jq &> /dev/null; then
    # Silently fail if jq not available
    exit 0
fi

# Extract port from manifest
PORT=$(jq -r '.port' "$MANIFEST_FILE" 2>/dev/null)

# Validate port is a number
if [[ "$PORT" =~ ^[0-9]+$ ]] && [ "$PORT" -gt 0 ] && [ "$PORT" -lt 65536 ]; then
    echo "hostfwd=tcp::${PORT}-:${PORT}"
fi
