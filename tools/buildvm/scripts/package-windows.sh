#!/usr/bin/env bash
set -euo pipefail

DISK_IMG="disk.img"
ARCH_FILE="disk-arch.txt"
HYPERV_SCRIPT="scripts/start-hyperv.ps1"
VM_CONFIG="vm-config.json"
RELEASE_VM_CONFIG="scripts/release-vm-config.json"

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

# Map architecture to Windows package name
case "$ARCH" in
    x86_64)
        PACKAGE_NAME="windows-x86_64"
        ;;
    aarch64)
        PACKAGE_NAME="windows-arm64"
        ;;
    *)
        echo "Error: Unknown architecture: $ARCH"
        exit 1
        ;;
esac

PACKAGE_DIR="package/$PACKAGE_NAME"
VHDX_FILE="$PACKAGE_DIR/alpine-vm.vhdx"
RELEASE_FILE="release/$PACKAGE_NAME.tar.xz"

echo "==> Creating Windows Hyper-V package: $PACKAGE_NAME"

# Create package directory
mkdir -p "$PACKAGE_DIR"

# Convert disk image to VHDX format
echo "==> Converting disk.img to VHDX format (this may take a few minutes)..."
qemu-img convert -f raw -O vhdx -o subformat=dynamic "$DISK_IMG" "$VHDX_FILE"

# Copy VM configuration with default values for release
echo "==> Copying VM configuration..."
cp "$RELEASE_VM_CONFIG" "$PACKAGE_DIR/vm-config.json"

# Copy PowerShell startup script
echo "==> Copying Hyper-V startup script..."
cp "$HYPERV_SCRIPT" "$PACKAGE_DIR/start.ps1"

# Create README with usage instructions
echo "==> Creating README..."
cat > "$PACKAGE_DIR/README.md" << 'EOF'
# Alpine Linux VM for Windows

This package contains an Alpine Linux VM configured to run on Windows using Hyper-V.

## Prerequisites

- Windows 10/11 Pro or Enterprise
- Hyper-V enabled: `Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All`

## Usage

Run PowerShell as Administrator and execute:
```powershell
.\start.ps1
```

With custom resources (positional parameters):
```powershell
.\start.ps1 4 4096
```

Or with named parameters:
```powershell
.\start.ps1 -ProcessorCount 4 -MemoryMB 4096
```

## VM Resource Configuration

VM resources (CPUs and memory) are configured in `vm-config.json`:
```json
{
  "cpus": 2,
  "memoryMB": 2048
}
```

You can also override these via command line parameters:
```powershell
.\start.ps1 -ProcessorCount <cpu_count> -MemoryMB <memory_mb>
```

Containers running inside the VM will automatically have access to all allocated resources.

## Connection

The script will display connection instructions. Typically:

1. Connect to VM console: `vmconnect.exe localhost 'Alpine-VM'`
2. Wait for boot (20-30 seconds)
3. Get VM IP: Run `ip addr show eth0` in VM console
4. SSH from Windows: `ssh -i vm-key alpine@<vm-ip-address>`

## Folder Sharing

Hyper-V does not support virtiofs. To share folders between Windows and the VM:

1. Create a Windows SMB share
2. Mount it in the VM using CIFS

See Hyper-V documentation for details on network file sharing.

## Management

- **Stop VM:** `Stop-VM -Name 'Alpine-VM'`
- **Remove VM:** `Remove-VM -Name 'Alpine-VM' -Force`
- **VM Console:** `vmconnect.exe localhost 'Alpine-VM'`
EOF

echo "==> README.md created"

# Get file sizes
DISK_SIZE=$(stat -f%z "$DISK_IMG" 2>/dev/null || stat -c%s "$DISK_IMG")
VHDX_SIZE=$(stat -f%z "$VHDX_FILE" 2>/dev/null || stat -c%s "$VHDX_FILE")

echo ""
echo "==> Package created successfully!"
echo "==> Package directory: $PACKAGE_DIR"
echo "==> VHDX file: $VHDX_FILE"
echo "==> Startup script: $PACKAGE_DIR/start.ps1"
echo "==> VM config: $PACKAGE_DIR/vm-config.json"
echo "==> README: $PACKAGE_DIR/README.md"
echo "==> Original disk size: $(numfmt --to=iec-i --suffix=B $DISK_SIZE 2>/dev/null || echo "$DISK_SIZE bytes")"
echo "==> VHDX size: $(numfmt --to=iec-i --suffix=B $VHDX_SIZE 2>/dev/null || echo "$VHDX_SIZE bytes")"

# Create compressed release archive
echo ""
echo "==> Creating compressed release archive..."
mkdir -p release

# Use tar to create archive and pipe to xz for compression
tar -C package -cf - "$PACKAGE_NAME" | xz -6 -v > "$RELEASE_FILE"

RELEASE_SIZE=$(stat -f%z "$RELEASE_FILE" 2>/dev/null || stat -c%s "$RELEASE_FILE")

echo ""
echo "=========================================="
echo "  Windows Package Ready for Distribution"
echo "=========================================="
echo ""
echo "Release file: $RELEASE_FILE"
echo "Size: $(numfmt --to=iec-i --suffix=B $RELEASE_SIZE 2>/dev/null || echo "$RELEASE_SIZE bytes")"
echo "Architecture: $ARCH"
echo ""
echo "To extract on Windows:"
echo "  tar -xf $PACKAGE_NAME.tar.xz"
echo "  cd $PACKAGE_NAME"
echo "  .\\start.ps1"
echo ""
