#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
MIN_SIZE_BUFFER_MB=100  # Extra space beyond minimum

if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task init-vm' first."
    exit 1
fi

echo "==> Checking if running as root..."
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run as root (use sudo)"
    exit 1
fi

echo "==> Attaching $DISK_IMG to loop device..."
LOOP_DEV=$(losetup -f --show -P "$DISK_IMG")
echo "==> Attached to $LOOP_DEV"

# Ensure cleanup on exit
cleanup() {
    echo "==> Detaching loop device..."
    losetup -d "$LOOP_DEV" 2>/dev/null || true
}
trap cleanup EXIT

# Wait for partition device nodes to appear
sleep 1
if [ ! -b "${LOOP_DEV}p2" ]; then
    echo "Error: Partition ${LOOP_DEV}p2 not found"
    exit 1
fi

echo "==> Checking and repairing filesystem..."
# e2fsck returns 0 (no errors) or 1 (errors corrected), both are success
# Exit codes 2+ indicate problems that need attention
set +e
e2fsck -f -y "${LOOP_DEV}p2"
E2FSCK_EXIT=$?
set -e
if [ $E2FSCK_EXIT -gt 1 ]; then
    echo "Error: e2fsck failed with exit code $E2FSCK_EXIT"
    exit 1
fi

echo "==> Shrinking filesystem to minimum size..."
resize2fs -M "${LOOP_DEV}p2"

echo "==> Getting filesystem size..."
BLOCK_COUNT=$(tune2fs -l "${LOOP_DEV}p2" | grep "^Block count:" | awk '{print $3}')
BLOCK_SIZE=$(tune2fs -l "${LOOP_DEV}p2" | grep "^Block size:" | awk '{print $3}')
FS_SIZE_MB=$((BLOCK_COUNT * BLOCK_SIZE / 1024 / 1024))
NEW_FS_SIZE_MB=$((FS_SIZE_MB + MIN_SIZE_BUFFER_MB))

echo "==> Filesystem minimum: ${FS_SIZE_MB}MB"
echo "==> Target size with buffer: ${NEW_FS_SIZE_MB}MB"

echo "==> Resizing filesystem to target size..."
resize2fs "${LOOP_DEV}p2" "${NEW_FS_SIZE_MB}M"

# Get current partition info
echo "==> Getting partition information..."
PART_START=$(parted -ms "$LOOP_DEV" unit s print | grep "^2:" | cut -d: -f2 | sed 's/s//')
SECTOR_SIZE=512  # Standard sector size

# Calculate new partition end
NEW_SIZE_BYTES=$((NEW_FS_SIZE_MB * 1024 * 1024))
NEW_SIZE_SECTORS=$((NEW_SIZE_BYTES / SECTOR_SIZE))
NEW_PART_END=$((PART_START + NEW_SIZE_SECTORS - 1))

echo "==> Partition starts at sector: $PART_START"
echo "==> New partition end sector: $NEW_PART_END"

echo "==> Detaching loop device before partition resize..."
losetup -d "$LOOP_DEV"
trap - EXIT  # Remove cleanup trap

echo "==> Resizing partition..."
parted ---pretend-input-tty "$DISK_IMG" <<EOF
resizepart 2
${NEW_PART_END}s
Yes
quit
EOF

# Calculate final disk size based on actual partition layout
# Disk needs to be at least: (last partition end + GPT backup table)
OVERHEAD_SECTORS=34  # Sectors needed for GPT backup table at end
FINAL_SIZE_SECTORS=$((NEW_PART_END + OVERHEAD_SECTORS + 1))
FINAL_SIZE_BYTES=$((FINAL_SIZE_SECTORS * SECTOR_SIZE))
FINAL_SIZE_MB=$((FINAL_SIZE_BYTES / 1024 / 1024 + 1))  # Round up

echo "==> Truncating disk image to ${FINAL_SIZE_MB}MB..."
truncate -s "${FINAL_SIZE_MB}M" "$DISK_IMG"

echo "==> Repairing GPT backup header..."
sgdisk -e "$DISK_IMG" > /dev/null 2>&1 || {
    echo "Warning: Failed to repair GPT, trying alternative method..."
    # Alternative: use gdisk
    gdisk "$DISK_IMG" <<EOF > /dev/null 2>&1 || true
x
e
w
Y
EOF
}

echo "==> Verifying disk integrity..."
parted "$DISK_IMG" print 2>&1 | grep -q "Partition Table: gpt" || {
    echo "Error: GPT partition table is corrupted"
    exit 1
}

echo "==> Disk shrinking complete!"
echo "==> Original size: 3GB"
echo "==> New size: ${FINAL_SIZE_MB}MB (~$(echo "scale=1; $FINAL_SIZE_MB / 1024" | bc)GB)"
echo "==> Saved: ~$((3072 - FINAL_SIZE_MB))MB"
touch disk-shrunk.flag
