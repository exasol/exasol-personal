# BuildVM Requirements

## Overview

BuildVM is a tool for building lightweight Alpine Linux VM images with embedded Podman container support. The tool produces VM packages for multiple platforms (macOS/vfkit, Windows/Hyper-V) from a single Alpine Linux base image.

The VM is branded as "Exasol VM" for end users, with technical references to Alpine Linux preserved in internal documentation. VM hostname is exasol-vm and the default user is exasol.

## Core Requirements

### R1: Multi-Platform Support

**R1.1**: The tool MUST support building VM images for:
- macOS (using vfkit/Virtualization.framework)
- Windows (using Hyper-V)
- Linux (QEMU for development/testing)

**R1.2**: Each platform package MUST include:
- VM disk image in platform-native format (raw for vfkit, VHDX for Hyper-V)
- Platform-specific startup script (bash for macOS, PowerShell for Windows)
- VM configuration file (vm-config.json)
- README with usage instructions

### R2: Multi-Architecture Support

**R2.1**: The tool MUST support both x86_64 and ARM64 architectures

**R2.2**: Architecture detection MUST be automatic by:
- Mounting the disk image via loopback
- Inspecting the kernel file (vmlinuz-*) with the `file` command
- Detecting "x86-64|x86_64" or "aarch64|ARM aarch64" patterns (case-insensitive)
- Writing normalized architecture to disk-arch.txt

**R2.3**: Package names MUST reflect architecture:
- mac-arm64 / mac-x86_64
- windows-arm64 / windows-x86_64

**R2.4**: QEMU configuration MUST be architecture-specific:
- x86_64: qemu-system-x86_64, q35 machine, qemu64 CPU, OVMF firmware
- ARM64: qemu-system-aarch64, virt machine, cortex-a72 CPU, QEMU_EFI.fd firmware

### R3: Base Image Requirements

**R3.1**: Base image MUST be Alpine Linux NoCloud UEFI image

**R3.2**: Image source MUST be cached locally as alpine-pristine.img to avoid re-downloading

**R3.3**: Disk image MUST be:
- Initially resized to 3GB
- Shrunk to minimum size + overhead after initialization
- Final size approximately 1-1.5GB depending on contents

### R4: Container Runtime

**R4.1**: VM MUST include Podman for container management

**R4.2**: Podman MUST run as root (not rootless) to avoid cgroup issues

**R4.3**: Container MUST have NO resource limits set (--cpus, --memory flags MUST NOT be used)

**R4.4**: Container MUST automatically inherit all VM CPU and memory resources

**R4.5**: Container MUST use host networking (--network host)

**R4.6**: Container MUST mount data directory: /mnt/host/container-data → /data

**R4.7**: Container logs MUST be written to shared folder: /mnt/host/logs/

### R5: Cloud-Init Integration

**R5.1**: VM MUST be initialized using cloud-init NoCloud datasource

**R5.2**: Cloud-init ISO MUST be presented with media=cdrom flag

**R5.3**: Cloud-init MUST configure:
- Hostname: exasol-vm
- User: exasol (with SSH key authentication)
- System timezone: UTC
- cgroup2 filesystem mounting
- Podman installation and configuration
- SSH key import from shared folder
- Container loading from shared folder
- GRUB timeout = 0
- Cloud-init service disablement after first boot

**R5.4**: Cloud-init MUST run custom scripts in order:
1. setup-system.sh (system configuration)
2. setup-services.sh (service configuration)
3. import-shared-keys.sh (SSH key import)
4. load-shared-container.sh (container loading)
5. cleanup-and-shutdown.sh (finalization)

**R5.5**: VM branding and naming MUST be consistent:
- All user-facing references MUST use "Exasol VM" branding (not "Alpine Linux VM")
- VM hostname: exasol-vm (not alpine-vm)
- Default user: exasol (not alpine)
- SSH key comment: exasol-vm-key (not alpine-vm-key)
- Console messages: "Starting Exasol VM" (not "Starting Alpine Linux VM")
- Task descriptions: "Start the Exasol VM" (not "Start the Alpine Linux VM")
- Hyper-V VM name: Exasol-VM (not Alpine-VM)
- vfkit VM name: Exasol-VM (not Alpine-VM)
- Technical documentation MAY reference Alpine Linux as the base OS
- Internal file names (alpine-pristine.img, alpine-cloud.qcow2) preserved for clarity

### R6: Folder Sharing

**R6.1**: Shared folder MUST be mounted at /mnt/host inside the VM

**R6.2**: virtiofs MUST be used with mount tag "hostshare" for:
- QEMU (development)
- vfkit (macOS)

**R6.3**: Hyper-V MUST use a secondary block device (VHDX) instead of virtiofs:
- virtiofs is not supported on Hyper-V
- Block device is attached as secondary drive and mounted at /mnt/host
- File: exasol-data.vhdx (10GB dynamic) created automatically

**R6.4**: vfkit startup script MUST accept shared directory as third positional argument

**R6.5**: Shared folder MUST be optional (VM can run without it)

**R6.6**: Platform detection MUST transparently handle storage:
- Try mounting virtiofs first (Mac/QEMU/vfkit)
- Fall back to block device if virtiofs unavailable (Hyper-V)
- Check for /dev/vdb, /dev/sdb, or /dev/xvdb
- Format block device as ext4 with label "exasol-data" if not formatted
- Mount at /mnt/host with "defaults,nofail" options
- Add to /etc/fstab for automatic mounting on subsequent boots
- Display warning if neither virtiofs nor block device available

**R6.7**: Data disk lifecycle:
- Created automatically by start.ps1 if it doesn't exist
- Persisted across VM restarts
- Attached to existing VMs if not already attached
- Formatted on first boot by cloud-init

### R7: Container Loading System

**R7.1**: Container manifest (container-manifest.json) MUST define:
- `containerFile`: Path to container tarball (optional, null = skip loading)
- `ports`: Array of port numbers that the container exposes (e.g., `[8080]` or `[8080, 3000]`)
- `args`: Array of command-line arguments
- `mounts`: Array of volume mounts (optional, no mounts created if not specified)
  - Each mount has `hostPath` (relative to /mnt/host) and `containerPath`
  - Paths containing `..` MUST be rejected with error
  - hostPath starting with `./` has prefix stripped (e.g., `./data` becomes `/mnt/host/data`)
  - Multiple mounts are supported

**R7.1.1**: Port validation MUST occur before init-vm:
- All ports listed in container manifest MUST be exposed in vm-config.json
- Validation script (validate-port-config.sh) checks container ports against vm-config.json
- init-vm MUST fail with clear error if validation fails
- Validation passes if no container manifest present (no ports to validate)
- Validation warning if vm-config.json has no ports field but container manifest exists

**R7.2**: Container loading MUST support:
- Initial load from tarball
- Skip reload if SHA256 checksum unchanged
- Force reload if container file changes
- Starting existing container if manifest missing/incomplete

**R7.3**: Container state MUST be tracked via SHA256 in /var/lib/container-state.sha256

**R7.4**: Container MUST be named "container" (hardcoded)

**R7.5**: Multiple containers MUST NOT be supported (single container only)

**R7.6**: Container loading MUST occur during:
- VM initialization (init-vm) via cloud-init - loads container into VM image
- Subsequent VM startups via load-shared-container service

**R7.7**: Container data directory (/mnt/host/container-data) MUST:
- Be automatically created by container loading script if missing
- Be mounted as /data inside the container
- Persist data across restarts when shared folder is available

**R7.8**: Containers MUST be tolerant of data folder volatility:
- Data folder may be missing if shared folder unavailable (Hyper-V without data disk)
- Data folder may be empty if user clears shared folder
- Container should handle missing/empty data gracefully (recreate defaults, skip optional features)

**R7.9**: Container manifest MUST be stored to /var/lib/container-manifest.json:
- Stored after successful container load from tarball
- Used as fallback when shared folder manifest unavailable
- Enables container restarts without shared folder access

**R7.10**: Port forwarding configuration:
- vm-config.json MUST define ports array for container port forwarding
- Each port entry MUST include: protocol ("tcp" or "udp"), host (port number), vm (port number)
- All three fields (protocol, host, vm) are REQUIRED - no defaults, no optional fields
- Empty ports array disables container port forwarding (SSH only)
- Format for CLI args: "protocol:host:vm,protocol:host:vm,..."
- Examples: "tcp:8080:8080", "tcp:8080:8080,tcp:9000:3000"
- Supports port remapping (host port ≠ VM port)
- No auto-detection fallback - ports MUST be explicitly configured

**R7.11**: Platform-specific port forwarding behavior:
- QEMU (Linux dev): Automatic forwarding via `-netdev user,hostfwd=protocol::host-:vm`
  - Container accessible at localhost:host_port
  - Uses host port (first number) from vm-config.json
- vfkit (macOS): Automatic forwarding via `--device virtio-net,nat,guestPort=vm,hostPort=host`
  - Container accessible at localhost:host_port
  - Uses host port (first number) from vm-config.json
  - CLI script accepts port_rules as positional arg 3
- Hyper-V (Windows): No automatic forwarding
  - Container accessible at vm_ip:vm_port
  - Uses VM port (second number) from vm-config.json
  - Host port (first number) ignored for direct IP access
  - CLI script accepts PortRules as positional parameter 2
  - VM IP written to vm-ip.txt file (see R7.12)

**R7.12**: Hyper-V VM IP detection and storage:
- start-hyperv.ps1 MUST attempt to detect VM IP address after startup
- Maximum wait time: 5 minutes (300 seconds)
- Poll interval: 2 seconds
- VM IP MUST be written to vm-ip.txt in same directory as data disk
- File format: Plain text, IPv4 address only, no newline
- If IP unavailable after timeout: Write error message to file
- VM IP displayed in console output when available
- Container access URLs shown with actual IP when available

**R7.13**: Shared folder lifecycle management:
- After init-vm completes, shared folder MUST be completely cleaned (all contents removed)
- Cleanup MUST be performed by clean-shared task (internal) called by init-vm task
- Cleaned items include: scripts/, test containers, manifests, authorized_keys, logs/, container-data/
- Rationale: Scripts copied into VM disk, containers loaded into VM image, no longer needed
- start-vm MUST recreate necessary files:
  - shared/authorized_keys from vm-key.pub (for SSH access)
- Container loading service MUST recreate necessary directories on demand:
  - shared/logs/ for container logs
  - shared/container-data/ for volume mounts (if specified in manifest)
- Tests operate on clean, empty shared folder with predictable state
- Container services MUST tolerate missing files and recreate as needed

### R8: SSH Access

**R8.1**: SSH MUST be accessible on port 2222 via port forwarding

**R8.2**: SSH MUST use key-based authentication

**R8.3**: SSH keys MUST be:
- Generated during build (vm-key, vm-key.pub)
- Automatically imported from /mnt/host/authorized_keys if present
- Managed via import-shared-keys service

**R8.4**: SSH MUST use exasol user (not root)

**R8.5**: SSH key security MUST enforce:
- Only keys in /mnt/host/authorized_keys have VM access
- All existing keys are replaced (not appended) on each import
- import-shared-keys service runs before sshd starts accepting connections

**R8.6**: Development workflow MUST:
- start-vm copies vm-key.pub to shared/authorized_keys
- connect uses vm-key for SSH access
- stop-vm removes shared/authorized_keys for security

### R9: Resource Configuration

**R9.1**: Default VM resources MUST be:
- 2 CPUs
- 2048 MB RAM

**R9.2**: init-vm-config.json MUST define build-time VM resources:
```json
{
  "cpus": 2,
  "memoryMB": 2048,
  "description": "VM build-time configuration (init-vm only). For runtime config, see vm-config.json."
}
```

**R9.3**: vm-config.json MUST define runtime VM resources and port forwarding:
```json
{
  "cpus": 2,
  "memoryMB": 2048,
  "description": "VM runtime configuration. Containers will automatically inherit these resources.",
  "ports": [
    {"protocol": "tcp", "host": 8080, "vm": 8080}
  ]
}
```

**R9.4**: Development environment uses:
- init-vm-config.json for VM build (task init-vm)
- vm-config.json for VM runtime (task start-vm)

**R9.5**: Release packages MUST include vm-config.json (copied from buildvm/vm-config.json)

**R9.6**: Startup scripts MUST accept resources as positional arguments:
- Position 0: CPU count
- Position 1: Memory in MB
- Position 2: Port rules (format: "protocol:host:vm,protocol:host:vm,...")
- Position 3: Shared folder path / Data disk path (platform-dependent)

**R9.7**: Startup scripts MUST NOT depend on JSON parsing (jq, ConvertFrom-Json)

**R9.8**: Both bash and PowerShell scripts MUST support positional parameters for easy launcher integration

### R10: Boot Optimization

**R10.1**: GRUB timeout MUST be set to 0 to skip boot menu

**R10.2**: GRUB configuration MUST be applied during cloud-init:
- Set GRUB_TIMEOUT=0
- Set GRUB_HIDDEN_TIMEOUT=0
- Regenerate grub.cfg via grub-mkconfig or update-grub

**R10.3**: VM MUST boot directly to Alpine Linux without user interaction

### R11: Disk Management

**R11.1**: Disk shrinking MUST:
- Resize ext4 filesystem to minimum size
- Calculate new partition size based on sector-based calculation
- Repair GPT backup header after truncation (sgdisk -e)
- Verify GPT integrity before completion
- Add 34 sectors overhead for GPT structures

**R11.2**: Disk compression MUST:
- Use xz compression level 6
- Keep original file (-k flag)
- Show progress (-v flag)
- Produce .img.xz file

**R11.3**: VHDX conversion MUST:
- Use dynamic allocation (subformat=dynamic)
- Convert from raw format
- Be performed by qemu-img

**R11.4**: Hyper-V data disk MUST:
- Be created as 10GB dynamically expanding VHDX
- Default filename: exasol-data.vhdx
- Created in script directory if no path specified
- Created automatically if it doesn't exist
- Persisted across VM lifecycle
- Attached to VM on creation or when missing

### R12: Network Configuration

**R12.1**: VM MUST use DHCP for network configuration

**R12.2**: Port forwarding MUST be configurable via manifest port field

**R12.3**: QEMU/vfkit MUST forward:
- Port 2222 → 22 (SSH)
- Container port (from manifest) → same port

**R12.4**: Hyper-V uses NAT by default; users find IP via console

### R13: Logging

**R13.1**: VM console output MUST be logged to:
- vm-init.log (during initialization)
- vm.log (during runtime)
- vm-console.log (vfkit)

**R13.2**: Container logs MUST be written to:
- /mnt/host/logs/container-runtime-YYYYMMDD-HHMMSS.log

**R13.3**: Container loading logs MUST include timestamps

**R13.4**: init-vm MUST tail VM log in real-time during initialization

**R13.5**: Background tail process MUST be cleaned up via trap on EXIT/INT/TERM

### R14: Build Workflow

**R14.1**: Build process MUST follow these steps in order:
1. Install dependencies (task install-deps)
2. Generate SSH key (task generate-ssh-key)
3. Download Alpine image (task download-image)
4. Detect architecture
5. Resize disk (task resize-disk)
6. Create cloud-init ISO (task create-cloud-init)
7. Initialize VM (task init-vm)
8. Shrink disk (task shrink-disk)
9. Run tests (task test)
10. Package for distribution (task package-mac / task package-windows)

**R14.2**: Build MUST be idempotent (can re-run from any step)

**R14.3**: Cleanup MUST remove all generated files and running processes

### R15: Testing

**R15.1**: Tests MUST verify:
- SSH key import from shared folder
- Container loading and runtime
- Network connectivity
- Data persistence across restarts

**R15.2**: Test suite MUST use shared directory for test containers and manifests

**R15.3**: Tests MUST be automated (task test-ssh-keys, task test-container)

### R16: Packaging

**R16.1**: macOS package MUST include:
- alpine-vm.img (raw disk)
- start.sh (vfkit startup script)
- vm-config.json (default: 2 CPUs, 2GB RAM)
- README.md

**R16.2**: Windows package MUST include:
- alpine-vm.vhdx (Hyper-V disk)
- start.ps1 (Hyper-V startup script)
- vm-config.json (default: 2 CPUs, 2GB RAM)
- README.md

**R16.3**: Packages MUST be compressed to .tar.xz format

**R16.4**: Release archives MUST be in release/ directory

**R16.5**: Package working directories MUST be in package/ directory

### R17: Startup Script Interface

**R17.1**: start.sh (vfkit) MUST:
- Check for vfkit installation
- Accept positional arguments: [cpu_count] [memory_mb] [shared_dir]
- Use defaults: 2 CPUs, 2048 MB
- Detect host architecture (arm64 vs x86_64)
- Locate UEFI bootloader
- Configure virtio devices (blk, net, rng, serial, fs)
- Run in background with PID tracking
- Display connection instructions

**R17.2**: start.ps1 (Hyper-V) MUST:
- Require Administrator privileges
- Check Hyper-V availability
- Accept positional arguments: [ProcessorCount] [MemoryMB] [DataDiskPath]
- Use defaults: 2 CPUs, 2048 MB, exasol-data.vhdx
- Create Generation 2 VM (UEFI)
- Disable Secure Boot
- Configure dynamic memory (512MB min, 4GB max)
- Create or use existing VM
- Create data disk VHDX (10GB dynamic) if it doesn't exist
- Attach data disk to VM (both new and existing VMs)
- Stop VM temporarily if running when attaching data disk
- Display connection instructions including data mount point

**R17.3**: Both scripts MUST:
- Support positional parameters for launcher compatibility
- Work without JSON parsing dependencies
- Only require their respective hypervisor

### R18: Development Requirements

**R18.1**: Development environment MUST use QEMU with:
- virtiofsd for folder sharing
- Background tail for log monitoring
- Attached mode for debugging (Ctrl-A X to exit)

**R18.2**: Development cycle MUST support:
- task start-vm (normal mode)
- task start-vm-attached (debugging mode)
- task stop-vm (graceful shutdown, 5 minute timeout)
- task connect (SSH to VM)

**R18.3**: Resource configuration MUST be customizable via vm-config.json in buildvm directory

### R19: Dependencies

**R19.1**: Build-time dependencies:
- qemu-system-x86 (x86_64 support)
- qemu-system-aarch64 (ARM64 support)
- qemu-utils (qemu-img)
- qemu-efi-aarch64 (ARM64 firmware)
- ovmf (x86_64 firmware)
- wget (image download)
- genisoimage (cloud-init ISO)
- parted (partition management)
- e2fsprogs (ext4 tools)
- bc (calculations)
- xz-utils (compression)
- virtiofsd (folder sharing)
- uidmap (user namespaces)
- podman (container building)
- jq (JSON parsing in QEMU scripts)
- gdisk (GPT repair)

**R19.2**: Runtime dependencies (development):
- QEMU system emulator
- virtiofsd

**R19.3**: Runtime dependencies (macOS release):
- vfkit

**R19.4**: Runtime dependencies (Windows release):
- Hyper-V
- PowerShell 5.1+

### R20: Configuration Files

**R20.1**: Taskfile.yml MUST define all build and test tasks

**R20.2**: build-config.yaml MUST define platform configurations:
- Image URLs
- Target OS
- Architecture mappings

**R20.3**: vm-config.json (buildvm root) MAY customize development resources

**R20.4**: scripts/release-vm-config.json MUST define default release resources

**R20.5**: .gitignore MUST exclude:
- Generated disk images
- VM runtime files (PIDs, sockets, logs)
- SSH keys
- Package and release directories
- Cloud-init ISO

### R21: Error Handling

**R21.1**: All scripts MUST use `set -euo pipefail` (bash) or `$ErrorActionPreference = "Stop"` (PowerShell)

**R21.2**: Missing dependencies MUST cause immediate failure with clear error message

**R21.3**: Architecture detection failure MUST cause build failure

**R21.4**: GPT corruption after shrinking MUST cause failure

**R21.5**: VM startup failure MUST be detected and reported

### R22: Documentation

**R22.1**: Each package MUST include README.md with:
- Prerequisites
- Basic usage
- Resource configuration
- Folder sharing (where applicable)
- Connection instructions
- Management commands
- Troubleshooting notes

**R22.2**: Main README.md MUST document:
- Build process
- Development workflow
- Testing procedures
- Packaging instructions

**R22.3**: TODO.md MUST track pending features and improvements

**R22.4**: requirements.md (this file) MUST be comprehensive enough to recreate the tool

### R23: Security

**R23.1**: VM MUST run with minimal privileges where possible

**R23.2**: SSH keys MUST be generated per-build (not reused)

**R23.3**: Secure Boot MUST be disabled (Alpine kernel not signed)

**R23.4**: Containers MUST run as root inside VM (isolated from host)

**R23.5**: SSH password authentication MUST be disabled

### R24: Performance

**R24.1**: VM MUST boot in under 30 seconds

**R24.2**: Container MUST start automatically on boot

**R24.3**: GRUB menu delay MUST be eliminated

**R24.4**: Disk image MUST be shrunk to minimum viable size

**R24.5**: QEMU MUST allocate 2 CPUs via -smp flag

**R24.6**: vfkit MUST allocate configurable CPUs via --cpus

**R24.7**: Hyper-V MUST allocate configurable CPUs via Set-VMProcessor

### R25: Compatibility

**R25.1**: Alpine Linux version MUST be 3.23.x (or compatible)

**R25.2**: QEMU MUST support q35/virt machines

**R25.3**: vfkit MUST support virtio-fs for folder sharing

**R25.4**: Hyper-V MUST support Generation 2 VMs

**R25.5**: PowerShell script MUST work on PowerShell 5.1+

**R25.6**: Bash script MUST work on bash 4.0+

## Non-Requirements

### NR1: Multiple Container Support
The system MUST NOT support running multiple containers simultaneously. Single container only.

### NR2: GUI Access
The system MUST NOT provide graphical console access. SSH only.

### NR3: Hyper-V Folder Sharing
The Windows/Hyper-V package MUST NOT implement automatic folder sharing. Users must configure SMB/CIFS manually.

### NR4: Windows Development Environment
The tool MUST NOT support development on Windows. Linux only for builds.

### NR5: Dynamic Resource Scaling
The system MUST NOT support hot-adding CPUs or memory. VM restart required.

### NR6: Container Orchestration
The system MUST NOT include Kubernetes, Docker Swarm, or other orchestration tools.

### NR7: Persistent Storage Beyond Container Data
The system MUST NOT provide mechanisms for additional persistent volumes. Only /data directory is persistent.

## Future Considerations

### F1: Separate Init/Runtime Shared Directories
Consider separating cloud-init shared directory from runtime shared directory for better security.

### F2: IP Remapping in Manifest
Allow manifest to specify IP address remapping for multi-VM deployments.

### F3: Custom Mount Configuration
Allow manifest to define additional mount points beyond /data.

### F4: Aggressive Log Rotation
Implement log rotation to prevent disk space exhaustion.

### F5: Authorized Keys Cleanup
Remove all SSH keys not included in shared directory.

### F6: Container Loading Documentation
Create comprehensive documentation for the container loading system.

### F7: Build Profiles
Implement named build profiles for different use cases (development, production, minimal).
