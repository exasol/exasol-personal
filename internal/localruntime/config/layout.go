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

	controlSocketFileName     = "control.sock"
	runtimeStateFileName      = "runtime.state"
	stopRequestFileName       = "stop.request"
	pidFileName               = "exanano.pid"
	machineIdentifierFileName = "machine-id.bin"
	layerDiskImageFileName    = "layer.img"
	consoleLogFileName        = "console.log"
	runnerLogFileName         = "runner.log"
	payloadExecutableFileName = "db.run"
	payloadChecksumFileName   = "db.run.sha256"
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
