#!/usr/bin/env pwsh
#Requires -RunAsAdministrator

<#
.SYNOPSIS
    Start Alpine Linux VM using Hyper-V
.DESCRIPTION
    Creates and starts an Alpine Linux VM in Hyper-V with the provided VHDX disk.
    Requires administrative privileges and Hyper-V to be enabled.
    
    NOTE: Containers running inside the VM will automatically have access to all
    VM resources (CPUs and memory). No additional configuration is needed.

.PARAMETER ProcessorCount
    Number of CPUs to allocate to the VM (default: 2)

.PARAMETER MemoryMB
    Memory in MB to allocate to the VM (default: 2048)

.PARAMETER SharedFolder
    Shared folder path (reserved for future use, currently ignored)

.PARAMETER VMName
    Name of the Hyper-V VM (default: Alpine-VM)

.PARAMETER VHDXPath
    Path to the VHDX disk file (default: alpine-vm.vhdx)

.PARAMETER SwitchName
    Name of the Hyper-V virtual switch (default: Default Switch)
#>

param(
    [int]$ProcessorCount = 2,
    [int]$MemoryMB = 2048,
    [string]$SharedFolder = "",
    [string]$VMName = "Alpine-VM",
    [string]$VHDXPath = "alpine-vm.vhdx",
    [string]$SwitchName = "Default Switch"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Convert memory from MB to bytes
$MemoryStartupBytes = $MemoryMB * 1MB

# Shared folder is not supported on Hyper-V yet, but accept the parameter
if ($SharedFolder) {
    Write-Warning "Shared folder parameter provided but not yet supported on Hyper-V. Use SMB/CIFS instead."
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
    
    Write-Host "==> VM created successfully!" -ForegroundColor Green
    Write-Host "==> Starting VM..." -ForegroundColor Cyan
    Start-VM -Name $VMName
}

# Wait a moment for VM to start
Start-Sleep -Seconds 2

# Get VM info
$vm = Get-VM -Name $VMName

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Alpine Linux VM Started Successfully" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "VM Information:" -ForegroundColor Cyan
Write-Host "  Name: $VMName"
Write-Host "  State: $($vm.State)"
Write-Host "  CPUs: $($vm.ProcessorCount)"
Write-Host "  Memory: $($MemoryStartupBytes / 1GB) GB"
Write-Host "  VHDX: $VHDXPath"
Write-Host "  Network: $SwitchName"
Write-Host ""
Write-Host "Connection Instructions:" -ForegroundColor Yellow
Write-Host ""
Write-Host "1. Connect to VM console:" -ForegroundColor White
Write-Host "   vmconnect.exe localhost '$VMName'" -ForegroundColor Gray
Write-Host ""
Write-Host "2. Wait for the VM to boot (20-30 seconds)" -ForegroundColor White
Write-Host ""
Write-Host "3. Login with:" -ForegroundColor White
Write-Host "   Username: alpine" -ForegroundColor Gray
Write-Host "   Password: <use SSH key or set password via cloud-init>" -ForegroundColor Gray
Write-Host ""
Write-Host "4. Get VM IP address (run in VM console):" -ForegroundColor White
Write-Host "   ip addr show eth0" -ForegroundColor Gray
Write-Host ""
Write-Host "5. SSH to VM from host:" -ForegroundColor White
Write-Host "   ssh -i vm-key alpine@<vm-ip-address>" -ForegroundColor Gray
Write-Host ""
Write-Host "Management Commands:" -ForegroundColor Yellow
Write-Host "  Stop VM:    Stop-VM -Name '$VMName'" -ForegroundColor Gray
Write-Host "  Remove VM:  Remove-VM -Name '$VMName' -Force" -ForegroundColor Gray
Write-Host "  VM Console: vmconnect.exe localhost '$VMName'" -ForegroundColor Gray
Write-Host ""
