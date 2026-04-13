#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
ARCH_FILE="disk-arch.txt"
VFKIT_SCRIPT="scripts/start-vfkit.sh"
VM_CONFIG="vm-config.json"

# Check if disk image exists
if [ ! -f "$DISK_IMG" ]; then
    echo "Error: $DISK_IMG not found. Run 'task build' first."
    exit 1
fi

# Check if architecture file exists
if [ ! -f "$ARCH_FILE" ]; then
    echo "Error: $ARCH_FILE not found. Cannot determine architecture."
    exit 1
fi

# Read architecture from file
ARCH=$(cat "$ARCH_FILE")
echo "==> Detected architecture: $ARCH"

# Map architecture to macOS package name
case "$ARCH" in
    x86_64)
        PACKAGE_NAME="mac-x86_64"
        ;;
    aarch64)
        PACKAGE_NAME="mac-arm64"
        ;;
    *)
        echo "Error: Unknown architecture: $ARCH"
        exit 1
        ;;
esac

PACKAGE_DIR="package/$PACKAGE_NAME"
DISK_FILE="$PACKAGE_DIR/alpine-vm.img"
RELEASE_FILE="release/$PACKAGE_NAME.tar.xz"

echo "==> Creating macOS vfkit package: $PACKAGE_NAME"

# Create package directory
mkdir -p "$PACKAGE_DIR"

# Copy disk image (vfkit works with raw disk images)
echo "==> Copying disk image for vfkit..."
cp "$DISK_IMG" "$DISK_FILE"

# Copy VM configuration with default values for release
echo "==> Copying VM configuration..."
cp "$VM_CONFIG" "$PACKAGE_DIR/vm-config.json"

# Copy vfkit startup script
echo "==> Copying vfkit startup script..."
cp "$VFKIT_SCRIPT" "$PACKAGE_DIR/start.sh"
chmod +x "$PACKAGE_DIR/start.sh"

# Create README with usage instructions
echo "==> Creating README..."
cat > "$PACKAGE_DIR/README.md" << 'EOF'
# Alpine Linux VM for macOS

This package contains an Alpine Linux VM configured to run on macOS using vfkit.

## Prerequisites

Install vfkit (virtualization framework wrapper):
```bash
brew install vfkit
```

## Basic Usage

Start the VM with default settings (2 CPUs, 2GB RAM):
```bash
./start.sh
```

Start with custom resources:
```bash
./start.sh 4 4096          # 4 CPUs, 4GB RAM
```

## Folder Sharing

To share a folder between your Mac and the VM, provide it as the third argument:
```bash
./start.sh 2 2048 /path/to/shared/folder
```

The shared folder will be mounted at `/mnt/host` inside the VM.

### Example with container deployment:

```bash
# Create a shared directory
mkdir shared

# Put your container tarball and manifest in it
cp my-container.tar.gz shared/
cp container-manifest.json shared/

# Start VM with shared folder (2 CPUs, 2GB RAM)
./start.sh 2 2048 shared
```

Inside the VM, your files will be at `/mnt/host`:
```bash
ssh -i vm-key -p 2222 alpine@localhost
ls /mnt/host
```

## Connection

- **SSH:** `ssh -i vm-key -p 2222 alpine@localhost`
- **Username:** alpine
- **Authentication:** SSH key (vm-key file in this directory)

## Management

- **Stop VM:** `killall vfkit`
- **View console:** `tail -f vm-console.log`
- **View vfkit log:** `tail -f vfkit.log`

## VM Resource Configuration

VM resources (CPUs and memory) are configured in `vm-config.json`:
```json
{
  "cpus": 2,
  "memoryMB": 2048
}
```

You can also override these via command line arguments:
```bash
./start.sh <cpu_count> <memory_mb> [shared_folder]
```

Containers running inside the VM will automatically have access to all allocated resources.

## Notes

- Wait 20-30 seconds for the VM to fully boot before connecting
- The VM runs in the background
- The virtiofs mount tag is "hostshare" (configured in cloud-init)
EOF

echo "==> README.md created"

# Get file sizes
DISK_SIZE=$(stat -f%z "$DISK_IMG" 2>/dev/null || stat -c%s "$DISK_IMG")

echo ""
echo "==> Package created successfully!"
echo "==> Package directory: $PACKAGE_DIR"
echo "==> Disk image: $DISK_FILE"
echo "==> Startup script: $PACKAGE_DIR/start.sh"
echo "==> VM config: $PACKAGE_DIR/vm-config.json"
echo "==> README: $PACKAGE_DIR/README.md"
echo "==> Disk size: $(numfmt --to=iec-i --suffix=B $DISK_SIZE 2>/dev/null || echo "$DISK_SIZE bytes")"

# Create compressed release archive
echo ""
echo "==> Creating compressed release archive..."
mkdir -p release

# Use tar to create archive and pipe to xz for compression
tar -C package -cf - "$PACKAGE_NAME" | xz -6 -v > "$RELEASE_FILE"

RELEASE_SIZE=$(stat -f%z "$RELEASE_FILE" 2>/dev/null || stat -c%s "$RELEASE_FILE")

echo ""
echo "=========================================="
echo "  macOS Package Ready for Distribution"
echo "=========================================="
echo ""
echo "Release file: $RELEASE_FILE"
echo "Size: $(numfmt --to=iec-i --suffix=B $RELEASE_SIZE 2>/dev/null || echo "$RELEASE_SIZE bytes")"
echo "Architecture: $ARCH"
echo ""
echo "To extract on macOS:"
echo "  tar -xf $PACKAGE_NAME.tar.xz"
echo "  cd $PACKAGE_NAME"
echo ""
echo "Basic usage:"
echo "  ./start.sh"
echo ""
echo "With folder sharing:"
echo "  ./start.sh /path/to/shared/folder"
echo ""
echo "See README.md in the package for more details"
echo ""
