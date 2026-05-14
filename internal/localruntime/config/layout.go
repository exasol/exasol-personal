// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import "path/filepath"

const (
	RuntimeRootDirName  = "local-runtime"
	stateFileName       = "state.json"
	configDirName       = "config"
	controlDirName      = "control"
	dataDirName         = "data"
	logDirName          = "logs"
	vmDirName           = "vm"
	payloadDirName      = "payload"
	bootstrapDirName    = "bootstrap"
	bootDirName         = "boot"
	guestPayloadDirName = "guest"
	hostShareDirName    = "host-share"

	controlSocketFileName       = "control.sock"
	runtimeStateFileName        = "runtime.state"
	stopRequestFileName         = "stop.request"
	pidFileName                 = "exanano.pid"
	machineIdentifierFileName   = "machine-id.bin"
	layerDiskImageFileName      = "layer.img"
	consoleLogFileName          = "console.log"
	runnerLogFileName           = "runner.log"
	payloadExecutableFileName   = "db.run"
	payloadChecksumFileName     = "db.run.sha256"
	machineSizingFileName       = "machine.json"
	efiVariableStoreFileName    = "efi-vars.bin"
	bootDiskImageFileName       = "boot-disk.img"
	containerImageArchiveName   = "container.tar.gz"
	containerManifestFileName   = "container-manifest.json"
	containerImageChecksumName  = "container.tar.gz.sha256"
)

// Layout describes the deployment-owned on-disk layout for local runtime state.
type Layout struct {
	deploymentDir string
}

func NewLayout(deploymentDir string) Layout {
	return Layout{deploymentDir: deploymentDir}
}

func (l Layout) DeploymentDir() string {
	return l.deploymentDir
}

func (l Layout) RuntimeRoot() string {
	return filepath.Join(l.deploymentDir, RuntimeRootDirName)
}

func (l Layout) ConfigDir() string {
	return filepath.Join(l.RuntimeRoot(), configDirName)
}

func (l Layout) ControlDir() string {
	return filepath.Join(l.RuntimeRoot(), controlDirName)
}

func (l Layout) DataDir() string {
	return filepath.Join(l.RuntimeRoot(), dataDirName)
}

func (l Layout) LogsDir() string {
	return filepath.Join(l.RuntimeRoot(), logDirName)
}

func (l Layout) VMDir() string {
	return filepath.Join(l.RuntimeRoot(), vmDirName)
}

func (l Layout) PayloadDir() string {
	return filepath.Join(l.VMDir(), payloadDirName)
}

func (l Layout) PayloadBootDir() string {
	return filepath.Join(l.PayloadDir(), bootDirName)
}

func (l Layout) PayloadShareDir() string {
	return filepath.Join(l.PayloadDir(), guestPayloadDirName)
}

func (l Layout) BootstrapDir() string {
	return filepath.Join(l.ConfigDir(), bootstrapDirName)
}

func (l Layout) StateFile() string {
	return filepath.Join(l.RuntimeRoot(), stateFileName)
}

func (l Layout) ControlSocketPath() string {
	return filepath.Join(l.ControlDir(), controlSocketFileName)
}

func (l Layout) RuntimeStatePath() string {
	return filepath.Join(l.ControlDir(), runtimeStateFileName)
}

func (l Layout) StopRequestPath() string {
	return filepath.Join(l.ControlDir(), stopRequestFileName)
}

func (l Layout) PIDFilePath() string {
	return filepath.Join(l.ControlDir(), pidFileName)
}

func (l Layout) MachineIdentifierFile() string {
	return filepath.Join(l.VMDir(), machineIdentifierFileName)
}

func (l Layout) LayerDiskImageFile() string {
	return filepath.Join(l.VMDir(), layerDiskImageFileName)
}

func (l Layout) ConsoleLogFile() string {
	return filepath.Join(l.LogsDir(), consoleLogFileName)
}

func (l Layout) RunnerLogFile() string {
	return filepath.Join(l.LogsDir(), runnerLogFileName)
}

func (l Layout) PayloadExecutablePath() string {
	return filepath.Join(l.PayloadShareDir(), payloadExecutableFileName)
}

func (l Layout) PayloadChecksumPath() string {
	return filepath.Join(l.PayloadShareDir(), payloadChecksumFileName)
}

func (l Layout) MachineSizingPath() string {
	return filepath.Join(l.ConfigDir(), machineSizingFileName)
}

// EFIVariableStoreFile is the persistent EFI variable store used by vz when
// booting a UEFI disk image. vz creates it on first boot and reuses it across
// restarts to preserve boot order and NVRAM state.
func (l Layout) EFIVariableStoreFile() string {
	return filepath.Join(l.VMDir(), efiVariableStoreFileName)
}

// BootDiskImagePath is the cached path of the UEFI disk image that vz boots
// from. The disk image itself is fetched into the global asset cache; this
// path is the deployment-local copy or symlink used at runtime.
func (l Layout) BootDiskImagePath() string {
	return filepath.Join(l.VMDir(), bootDiskImageFileName)
}

// HostShareDir is the host-side directory that is mounted into the guest as a
// single virtio-fs share with the tag "hostshare". The guest's
// load-shared-container service reads its container tarball and manifest from
// this share. The directory lives under the deployment runtime root.
func (l Layout) HostShareDir() string {
	return filepath.Join(l.VMDir(), hostShareDirName)
}

// ContainerImageArchivePath is the host-side path of the staged container
// image tarball that the guest's loader will read from the hostshare mount.
func (l Layout) ContainerImageArchivePath() string {
	return filepath.Join(l.HostShareDir(), containerImageArchiveName)
}

// ContainerManifestPath is the host-side path of the guest container manifest
// (the JSON declaring containerFile/ports/args/mounts/shmSize).
func (l Layout) ContainerManifestPath() string {
	return filepath.Join(l.HostShareDir(), containerManifestFileName)
}

// ContainerImageChecksumPath is the host-side path that records the SHA256 of
// the staged container tarball so the guest can detect changes.
func (l Layout) ContainerImageChecksumPath() string {
	return filepath.Join(l.HostShareDir(), containerImageChecksumName)
}
