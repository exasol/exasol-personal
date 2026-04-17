#!/usr/bin/env bash
set -euo pipefail

ARCH_FILE="disk-arch.txt"

# Read architecture from file, default to aarch64 if not found
if [ -f "$ARCH_FILE" ]; then
    ARCH=$(cat "$ARCH_FILE")
else
    echo "Warning: $ARCH_FILE not found, defaulting to aarch64" >&2
    ARCH="aarch64"
fi

case "$ARCH" in
    x86_64)
        QEMU_BIN="qemu-system-x86_64"
        QEMU_MACHINE="q35"
        QEMU_CPU="max"
        
        # Check for OVMF firmware (multiple possible paths)
        QEMU_BIOS=""
        for path in "/usr/share/ovmf/OVMF.fd" "/usr/share/OVMF/OVMF_CODE.fd" "/usr/share/edk2/ovmf/OVMF_CODE.fd" "/usr/share/qemu/ovmf-x86_64.bin"; do
            if [ -f "$path" ]; then
                QEMU_BIOS="$path"
                break
            fi
        done
        
        if [ -z "$QEMU_BIOS" ]; then
            echo "Error: OVMF firmware not found. Install with: sudo apt-get install ovmf" >&2
            exit 1
        fi
        ;;
        
    aarch64)
        QEMU_BIN="qemu-system-aarch64"
        QEMU_MACHINE="virt"
        QEMU_CPU="cortex-a72"
        QEMU_BIOS="/usr/share/qemu-efi-aarch64/QEMU_EFI.fd"
        
        if [ ! -f "$QEMU_BIOS" ]; then
            echo "Error: ARM64 UEFI firmware not found. Install with: sudo apt-get install qemu-efi-aarch64" >&2
            exit 1
        fi
        ;;
        
    *)
        echo "Error: Unknown architecture: $ARCH" >&2
        exit 1
        ;;
esac

# Export variables for sourcing
export QEMU_BIN
export QEMU_MACHINE
export QEMU_CPU
export QEMU_BIOS

# Also print for debugging
echo "==> Using architecture: $ARCH" >&2
echo "==> QEMU binary: $QEMU_BIN" >&2
echo "==> Machine type: $QEMU_MACHINE" >&2
echo "==> CPU type: $QEMU_CPU" >&2
echo "==> BIOS/Firmware: $QEMU_BIOS" >&2
