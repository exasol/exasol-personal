#!/usr/bin/env bash
# Read container manifest and output QEMU port forwarding arguments

MANIFEST_FILE="shared/container-manifest.json"

# Exit silently if manifest doesn't exist
if [ ! -f "$MANIFEST_FILE" ]; then
    exit 0
fi

# Check if jq is installed (should be on host for this script)
if ! command -v jq &> /dev/null; then
    # Silently fail if jq not available
    exit 0
fi

# Read ports array and build forwarding rules
PORT_COUNT=$(jq -r '.ports // [] | length' "$MANIFEST_FILE" 2>/dev/null)

if [ "$PORT_COUNT" -eq 0 ]; then
    exit 0
fi

RULES=""

for i in $(seq 0 $((PORT_COUNT - 1))); do
    PORT=$(jq -r ".ports[$i]" "$MANIFEST_FILE" 2>/dev/null)
    
    # Validate port is a number
    if [[ "$PORT" =~ ^[0-9]+$ ]] && [ "$PORT" -gt 0 ] && [ "$PORT" -lt 65536 ]; then
        if [ -n "$RULES" ]; then
            RULES="$RULES,hostfwd=tcp::${PORT}-:${PORT}"
        else
            RULES="hostfwd=tcp::${PORT}-:${PORT}"
        fi
    fi
done

echo "$RULES"
