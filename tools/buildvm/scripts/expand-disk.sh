#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
TARGET_SIZE="3G"

if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task download-image' first."
    exit 1
fi

echo "==> Resizing disk image to $TARGET_SIZE..."
qemu-img resize "$DISK_IMG" "$TARGET_SIZE"

echo "==> Disk resized successfully."
touch disk-expanded.flag
