#!/usr/bin/env bash
# Validate that all container ports are exposed in vm-config.json

set -euo pipefail

MANIFEST_FILE="shared/container-manifest.json"
VM_CONFIG="vm-config.json"

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required for port validation"
    exit 1
fi

# Exit successfully if no container manifest (no validation needed)
if [ ! -f "$MANIFEST_FILE" ]; then
    exit 0
fi

# Exit successfully if no vm-config.json (will use defaults)
if [ ! -f "$VM_CONFIG" ]; then
    echo "Warning: $VM_CONFIG not found, skipping port validation"
    exit 0
fi

# Get container ports from manifest
CONTAINER_PORTS_ARRAY=$(jq -r '.ports // []' "$MANIFEST_FILE" 2>/dev/null)

if [ "$CONTAINER_PORTS_ARRAY" = "[]" ]; then
    # No ports specified in manifest
    exit 0
fi

# Read ports array
CONTAINER_PORTS=($(jq -r '.ports[]' "$MANIFEST_FILE" 2>/dev/null))

# Check if vm-config.json has ports field
HAS_PORTS=$(jq -r 'has("ports")' "$VM_CONFIG" 2>/dev/null || echo "false")

if [ "$HAS_PORTS" = "false" ]; then
    echo "Warning: vm-config.json does not have 'ports' field"
    echo "Container ports will be auto-detected from manifest (backward compatibility)"
    exit 0
fi

# Get VM ports from vm-config.json
VM_PORTS=($(jq -r '.ports[]?.vm // empty' "$VM_CONFIG" 2>/dev/null))

# Validate each container port is in VM ports
MISSING_PORTS=()

for container_port in "${CONTAINER_PORTS[@]}"; do
    FOUND=false
    for vm_port in "${VM_PORTS[@]}"; do
        if [ "$container_port" = "$vm_port" ]; then
            FOUND=true
            break
        fi
    done
    
    if [ "$FOUND" = "false" ]; then
        MISSING_PORTS+=($container_port)
    fi
done

# Report validation results
if [ ${#MISSING_PORTS[@]} -gt 0 ]; then
    echo "Error: Container ports not exposed in vm-config.json"
    echo ""
    echo "Container manifest specifies ports: ${CONTAINER_PORTS[*]}"
    echo "vm-config.json exposes ports: ${VM_PORTS[*]}"
    echo ""
    echo "Missing ports in vm-config.json: ${MISSING_PORTS[*]}"
    echo ""
    echo "Please update vm-config.json to include all container ports:"
    echo "{"
    echo "  \"cpus\": 2,"
    echo "  \"memoryMB\": 2048,"
    echo "  \"ports\": ["
    
    for i in "${!CONTAINER_PORTS[@]}"; do
        port="${CONTAINER_PORTS[$i]}"
        if [ $i -eq $((${#CONTAINER_PORTS[@]} - 1)) ]; then
            echo "    {\"protocol\": \"tcp\", \"host\": $port, \"vm\": $port}"
        else
            echo "    {\"protocol\": \"tcp\", \"host\": $port, \"vm\": $port},"
        fi
    done
    
    echo "  ]"
    echo "}"
    echo ""
    exit 1
fi

echo "==> Port validation passed: All container ports are exposed in vm-config.json"
exit 0
