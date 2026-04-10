#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
COMPRESSED_IMG="disk.img.xz"

if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task shrink-disk' first."
    exit 1
fi

if [ -f "$COMPRESSED_IMG" ]; then
    echo "==> Removing existing $COMPRESSED_IMG..."
    rm "$COMPRESSED_IMG"
fi

echo "==> Compressing $DISK_IMG with xz (this may take a few minutes)..."
echo "==> Using compression level 6 (balanced speed/size)..."

# Use xz with level 6 (default) for good compression without excessive time
# -k keeps the original file, -v shows progress
xz -6 -k -v "$DISK_IMG"

ORIGINAL_SIZE=$(stat -f%z "$DISK_IMG" 2>/dev/null || stat -c%s "$DISK_IMG")
COMPRESSED_SIZE=$(stat -f%z "$COMPRESSED_IMG" 2>/dev/null || stat -c%s "$COMPRESSED_IMG")

echo ""
echo "==> Compression complete!"
echo "==> Original size: $(numfmt --to=iec-i --suffix=B $ORIGINAL_SIZE 2>/dev/null || echo "$ORIGINAL_SIZE bytes")"
echo "==> Compressed size: $(numfmt --to=iec-i --suffix=B $COMPRESSED_SIZE 2>/dev/null || echo "$COMPRESSED_SIZE bytes")"
echo "==> Compression ratio: $(echo "scale=1; 100 - ($COMPRESSED_SIZE * 100 / $ORIGINAL_SIZE)" | bc)%"
echo ""
echo "==> Distribution file ready: $COMPRESSED_IMG"
