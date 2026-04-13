#!/usr/bin/env pwsh
#Requires -RunAsAdministrator

<#
.SYNOPSIS
    Start Linux VM using Hyper-V
.DESCRIPTION
    Creates and starts an Linux VM in Hyper-V with the provided VHDX disk.
    Requires administrative privileges and Hyper-V to be enabled.
    
    NOTE: Containers running inside the VM will automatically have access to all
    VM resources (CPUs and memory). No additional configuration is needed.

.PARAMETER ProcessorCount
    Number of CPUs to allocate to the VM (default: 2)

.PARAMETER MemoryMB
    Memory in MB to allocate to the VM (default: 2048)

.PARAMETER PortRules
    Port forwarding rules in format "protocol:host:vm,protocol:host:vm,..."
    Example: "tcp:8080:8080,tcp:9000:3000"
    Note: Hyper-V port forwarding requires manual NAT configuration. See output for instructions.

.PARAMETER DataDiskPath
    Path to data disk VHDX file. If empty, creates 'exasol-data.vhdx' in script directory.
    The data disk is mounted at /mnt/host inside the VM (default: exasol-data.vhdx)

.PARAMETER VMName
    Name of the Hyper-V VM (default: Exasol-VM)

.PARAMETER VHDXPath
    Path to the VHDX disk file (default: exasol-vm.vhdx)

.PARAMETER SwitchName
    Name of the Hyper-V virtual switch (default: Default Switch)

.EXAMPLE
    .\start.ps1
    .\start.ps1 4 4096
    .\start.ps1 2 2048 "tcp:8080:8080"
    .\start.ps1 2 2048 "tcp:8080:8080,tcp:9000:3000" "D:\vm-data\exasol-data.vhdx"
#>

param(
    [Parameter(Position=0)]
    [int]$ProcessorCount = 2,
    
    [Parameter(Position=1)]
    [int]$MemoryMB = 2048,
    
    [Parameter(Position=2)]
    [string]$PortRules = "",
    
    [Parameter(Position=3)]
    [string]$DataDiskPath = "",
    
    [string]$VMName = "Exasol-VM",
    [string]$VHDXPath = "exasol-vm.vhdx",
    [string]$SwitchName = "Default Switch"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Convert memory from MB to bytes
$MemoryStartupBytes = $MemoryMB * 1MB

# Setup data disk path (default to exasol-data.vhdx in script directory)
if (-not $DataDiskPath) {
    $DataDiskPath = Join-Path $PSScriptRoot "exasol-data.vhdx"
} elseif (-not [System.IO.Path]::IsPathRooted($DataDiskPath)) {
    $DataDiskPath = Join-Path $PSScriptRoot $DataDiskPath
}

# Ensure data disk directory exists
$DataDiskDir = Split-Path -Parent $DataDiskPath
if (-not (Test-Path $DataDiskDir)) {
    New-Item -ItemType Directory -Path $DataDiskDir -Force | Out-Null
}

# Create data disk if it doesn't exist (10GB dynamic VHDX)
if (-not (Test-Path $DataDiskPath)) {
    Write-Host "==> Creating data disk: $DataDiskPath" -ForegroundColor Cyan
    New-VHD -Path $DataDiskPath -SizeBytes 10GB -Dynamic | Out-Null
    Write-Host "==> Data disk created (10GB dynamic)" -ForegroundColor Green
} else {
    Write-Host "==> Using existing data disk: $DataDiskPath" -ForegroundColor Cyan
}

# Check if Hyper-V is available
try {
    $hyperv = Get-WindowsOptionalFeature -FeatureName Microsoft-Hyper-V-All -Online -ErrorAction Stop
    if ($hyperv.State -ne "Enabled") {
        Write-Error "Hyper-V is not enabled. Enable it via: Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All"
        exit 1
    }
} catch {
    Write-Error "Failed to check Hyper-V status. Ensure you're running Windows 10/11 Pro or Enterprise."
    exit 1
}

# Resolve VHDX path to absolute
if (-not [System.IO.Path]::IsPathRooted($VHDXPath)) {
    $VHDXPath = Join-Path $PSScriptRoot $VHDXPath
}

if (-not (Test-Path $VHDXPath)) {
    Write-Error "VHDX file not found: $VHDXPath"
    Write-Host "Make sure to run 'task package-windows' first to generate the VHDX file."
    exit 1
}

Write-Host "==> Using VHDX: $VHDXPath" -ForegroundColor Cyan

# Check if VM already exists
$existingVM = Get-VM -Name $VMName -ErrorAction SilentlyContinue

if ($existingVM) {
    Write-Host "==> VM '$VMName' already exists" -ForegroundColor Yellow
    
    # Check if data disk is attached
    $dataDisks = Get-VMHardDiskDrive -VMName $VMName | Where-Object { $_.Path -eq $DataDiskPath }
    if (-not $dataDisks) {
        Write-Host "==> Attaching data disk to existing VM..." -ForegroundColor Cyan
        if ($existingVM.State -eq "Running") {
            Write-Host "==> Stopping VM to attach data disk..." -ForegroundColor Yellow
            Stop-VM -Name $VMName -Force
            Start-Sleep -Seconds 2
        }
        Add-VMHardDiskDrive -VMName $VMName -Path $DataDiskPath
        Write-Host "==> Data disk attached successfully" -ForegroundColor Green
    }
    
    # Check VM state
    if ($existingVM.State -eq "Running") {
        Write-Host "==> VM is already running" -ForegroundColor Green
        Write-Host ""
        Write-Host "VM Information:" -ForegroundColor Cyan
        Write-Host "  Name: $VMName"
        Write-Host "  State: Running"
        Write-Host "  Memory: $($existingVM.MemoryAssigned / 1GB) GB"
        Write-Host ""
        Write-Host "To connect to the VM:" -ForegroundColor Yellow
        Write-Host "  1. Use Hyper-V Manager: vmconnect.exe localhost '$VMName'"
        Write-Host "  2. Once booted, SSH via: ssh -i vm-key alpine@<vm-ip-address>"
        Write-Host "     (Find IP by connecting to VM console and running: ip addr)"
        exit 0
    }
    
    Write-Host "==> Starting existing VM..." -ForegroundColor Cyan
    Start-VM -Name $VMName
} else {
    Write-Host "==> Creating new Hyper-V VM: $VMName" -ForegroundColor Cyan
    
    # Check if switch exists, create or use default
    $switch = Get-VMSwitch -Name $SwitchName -ErrorAction SilentlyContinue
    if (-not $switch) {
        Write-Host "==> Switch '$SwitchName' not found. Available switches:" -ForegroundColor Yellow
        Get-VMSwitch | Format-Table Name, SwitchType -AutoSize
        
        # Try to use Default Switch
        $switch = Get-VMSwitch -Name "Default Switch" -ErrorAction SilentlyContinue
        if ($switch) {
            $SwitchName = "Default Switch"
            Write-Host "==> Using 'Default Switch'" -ForegroundColor Green
        } else {
            # Create an internal switch
            Write-Host "==> Creating new internal switch: Alpine-Switch" -ForegroundColor Cyan
            New-VMSwitch -Name "Alpine-Switch" -SwitchType Internal | Out-Null
            $SwitchName = "Alpine-Switch"
        }
    }
    
    # Create the VM (Generation 2 for UEFI support)
    Write-Host "==> Creating Generation 2 VM (UEFI)..." -ForegroundColor Cyan
    New-VM -Name $VMName `
           -Generation 2 `
           -MemoryStartupBytes $MemoryStartupBytes `
           -VHDPath $VHDXPath `
           -SwitchName $SwitchName | Out-Null
    
    # Configure VM settings
    Write-Host "==> Configuring VM settings..." -ForegroundColor Cyan
    
    # Disable Secure Boot (Alpine may not have signed bootloader)
    Set-VMFirmware -VMName $VMName -EnableSecureBoot Off
    
    # Set processor count
    Set-VMProcessor -VMName $VMName -Count $ProcessorCount
    
    # Enable dynamic memory
    Set-VMMemory -VMName $VMName -DynamicMemoryEnabled $true -MinimumBytes 512MB -MaximumBytes 4GB
    
    # Configure automatic start/stop
    Set-VM -Name $VMName -AutomaticStartAction Nothing -AutomaticStopAction ShutDown
    
    # Attach data disk to VM
    Write-Host "==> Attaching data disk..." -ForegroundColor Cyan
    Add-VMHardDiskDrive -VMName $VMName -Path $DataDiskPath
    Write-Host "==> Data disk attached successfully" -ForegroundColor Green
    
    Write-Host "==> VM created successfully!" -ForegroundColor Green
    Write-Host "==> Starting VM..." -ForegroundColor Cyan
    Start-VM -Name $VMName
}

# Wait a moment for VM to start
Start-Sleep -Seconds 2

# Get VM info
$vm = Get-VM -Name $VMName

# Try to get VM IP address (may take time for DHCP)
Write-Host "==> Waiting for VM to obtain IP address (max 5 minutes)..." -ForegroundColor Cyan
$vmIP = $null
$maxWaitSeconds = 300  # 5 minutes
$waitedSeconds = 0

while ($waitedSeconds -lt $maxWaitSeconds) {
    $networkAdapter = Get-VMNetworkAdapter -VMName $VMName
    if ($networkAdapter.IPAddresses) {
        # Get first IPv4 address
        $vmIP = $networkAdapter.IPAddresses | Where-Object { $_ -match '^\d+\.\d+\.\d+\.\d+$' } | Select-Object -First 1
        if ($vmIP) {
            break
        }
    }
    Start-Sleep -Seconds 2
    $waitedSeconds += 2
}

# Write IP to file in data disk directory
$ipFilePath = Join-Path (Split-Path -Parent $DataDiskPath) "vm-ip.txt"
if ($vmIP) {
    $vmIP | Out-File -FilePath $ipFilePath -Encoding ASCII -NoNewline
    Write-Host "==> VM IP address: $vmIP" -ForegroundColor Green
    Write-Host "==> IP written to: $ipFilePath" -ForegroundColor Cyan
} else {
    Write-Host "==> Could not obtain VM IP address (VM may still be booting)" -ForegroundColor Yellow
    Write-Host "==> Check later with: Get-VM '$VMName' | Select -ExpandProperty NetworkAdapters | Select IPAddresses" -ForegroundColor Yellow
    "IP not available yet - check VM network adapter" | Out-File -FilePath $ipFilePath -Encoding ASCII
}

Write-Host ""
Write-Host "=========================================" -ForegroundColor Green
Write-Host "  Alpine Linux VM Started Successfully" -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
Write-Host ""
Write-Host "VM Information:" -ForegroundColor Cyan
Write-Host "  Name: $VMName"
Write-Host "  State: $($vm.State)"
Write-Host "  CPUs: $($vm.ProcessorCount)"
Write-Host "  Memory: $($MemoryStartupBytes / 1GB) GB"
Write-Host "  System VHDX: $VHDXPath"
Write-Host "  Data VHDX: $DataDiskPath"
Write-Host "  Network: $SwitchName"
if ($vmIP) {
    Write-Host "  VM IP: $vmIP (saved to vm-ip.txt)" -ForegroundColor Green
}
Write-Host ""

# Display port forwarding information if configured
if ($PortRules) {
    Write-Host "Container Port Access (Hyper-V):" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Note: On Hyper-V, connect using the VM's IP address and VM ports." -ForegroundColor Cyan
    Write-Host "Host port mappings (first number) only apply to optional NetNat configuration." -ForegroundColor Cyan
    Write-Host ""
    
    if ($vmIP) {
        Write-Host "VM IP: $vmIP" -ForegroundColor White
        Write-Host ""
        Write-Host "Access container ports:" -ForegroundColor White
        $portPairs = $PortRules -split ','
        foreach ($pair in $portPairs) {
            $parts = $pair -split ':'
            if ($parts.Length -eq 3) {
                $protocol = $parts[0].ToUpper()
                $vmPort = $parts[2]
                Write-Host "  $protocol port $vmPort -> http://${vmIP}:${vmPort}" -ForegroundColor Gray
            }
        }
    } else {
        Write-Host "Configured VM ports (use VM IP when available):" -ForegroundColor White
        $portPairs = $PortRules -split ','
        foreach ($pair in $portPairs) {
            $parts = $pair -split ':'
            if ($parts.Length -eq 3) {
                $protocol = $parts[0].ToUpper()
                $vmPort = $parts[2]
                Write-Host "  $protocol port $vmPort -> http://<vm-ip>:${vmPort}" -ForegroundColor Gray
            }
        }
        Write-Host ""
        Write-Host "Get IP from: $ipFilePath" -ForegroundColor Cyan
    }
    Write-Host ""
    Write-Host "Optional: For localhost access, configure NetNat manually:" -ForegroundColor Cyan
    Write-Host "  Example: Add-NetNatStaticMapping -NatName 'YourNAT' -Protocol TCP -ExternalPort <host-port> -InternalIPAddress $vmIP -InternalPort <vm-port>" -ForegroundColor Gray
    Write-Host ""
}

Write-Host "Management Commands:" -ForegroundColor Yellow
Write-Host "  Stop VM:    Stop-VM -Name '$VMName'" -ForegroundColor Gray
Write-Host "  Remove VM:  Remove-VM -Name '$VMName' -Force" -ForegroundColor Gray
Write-Host "  VM Console: vmconnect.exe localhost '$VMName'" -ForegroundColor Gray
Write-Host ""
