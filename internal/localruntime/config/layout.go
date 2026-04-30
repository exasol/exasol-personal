// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import "path/filepath"

const (
	RuntimeRootDirName = "local-runtime"
	stateFileName      = "state.json"
	configDirName      = "config"
	controlDirName     = "control"
	dataDirName        = "data"
	logDirName         = "logs"
	vmDirName          = "vm"

	payloadShareDirName = "payload-share"

	controlSocketFileName     = "control.sock"
	runtimeStateFileName      = "runtime.state"
	stopRequestFileName       = "stop.request"
	pidFileName               = "exanano.pid"
	machineIdentifierFileName = "machine-id.bin"
	diskImageFileName         = "disk.img"
	efiVarsFileName           = "efi-vars.fd"
	diskIdentityFileName      = "disk.identity"
	consoleLogFileName        = "console.log"
	runnerLogFileName         = "runner.log"
	machineSizingFileName     = "machine.json"
	payloadRunFileName        = "db.run"
	payloadStartScriptName    = "start.sh"
	payloadRunChecksumName    = ".db.run.sha256"
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

func (l Layout) DiskImagePath() string {
	return filepath.Join(l.VMDir(), diskImageFileName)
}

func (l Layout) EFIVarsPath() string {
	return filepath.Join(l.VMDir(), efiVarsFileName)
}

func (l Layout) DiskIdentityPath() string {
	return filepath.Join(l.VMDir(), diskIdentityFileName)
}

func (l Layout) ConsoleLogFile() string {
	return filepath.Join(l.LogsDir(), consoleLogFileName)
}

func (l Layout) RunnerLogFile() string {
	return filepath.Join(l.LogsDir(), runnerLogFileName)
}

func (l Layout) MachineSizingPath() string {
	return filepath.Join(l.ConfigDir(), machineSizingFileName)
}

func (l Layout) PayloadShareDir() string {
	return filepath.Join(l.VMDir(), payloadShareDirName)
}

func (l Layout) PayloadRunPath() string {
	return filepath.Join(l.PayloadShareDir(), payloadRunFileName)
}

func (l Layout) PayloadStartScriptPath() string {
	return filepath.Join(l.PayloadShareDir(), payloadStartScriptName)
}

func (l Layout) PayloadRunChecksumPath() string {
	return filepath.Join(l.PayloadShareDir(), payloadRunChecksumName)
}
