// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"io"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	dirName        = "local"
	runtimeDirName = "runtime"

	vmDirName          = "vm"
	sharedDirName      = "vm-shared"
	vmStateFileName    = "vm-state.json"
	PrivateKeyFileName = "node_access.pem"
	// runnerVersionMarkerFileName records the semver of the runner this
	// deployment was last prepared/started with. It's a launcher-owned file,
	// distinct from vm-state.json, whose schema is dictated by the runner's
	// own external contract and isn't ours to extend.
	runnerVersionMarkerFileName = "runner-version.json"
)

type VMConfig struct {
	CPUCount   int
	MemoryMB   int
	DataSizeGB int
	Ports      string
}

type RuntimeStatus struct {
	Running bool `json:"running"`
}

// Generic local runtime interface.
type Runtime interface {
	Deployment() config.DeploymentDir
	Prepare(ctx context.Context, out, outErr io.Writer) error
	Start(ctx context.Context, out, outErr io.Writer, runtimeConfig VMConfig) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	Status(ctx context.Context) (*RuntimeStatus, error)
	Destroy(ctx context.Context, out, outErr io.Writer) error
}

type runtimePaths struct {
	Root    string
	WorkDir string
}

func newRuntimePaths(deployment config.DeploymentDir) runtimePaths {
	root := deployment.Resolve(dirName)
	workDir := filepath.Join(root, runtimeDirName)

	return runtimePaths{
		Root:    root,
		WorkDir: workDir,
	}
}

// VM-based local runtime interface.
type VMRuntime interface {
	Runtime

	ReadState() (*State, error)
	HealthCheck(ctx context.Context) (*HealthCheckResult, error)
}

type vmRuntimePaths struct {
	runtimePaths

	VMDir                   string
	SharedDir               string
	StatePath               string
	PrivateKeyPath          string
	RunnerVersionMarkerPath string
}

func newVMRuntimePaths(deployment config.DeploymentDir) vmRuntimePaths {
	rtPaths := newRuntimePaths(deployment)

	return vmRuntimePaths{
		runtimePaths:            rtPaths,
		VMDir:                   filepath.Join(rtPaths.WorkDir, vmDirName),
		SharedDir:               filepath.Join(rtPaths.WorkDir, sharedDirName),
		StatePath:               filepath.Join(rtPaths.WorkDir, vmStateFileName),
		PrivateKeyPath:          filepath.Join(rtPaths.Root, PrivateKeyFileName),
		RunnerVersionMarkerPath: filepath.Join(rtPaths.WorkDir, runnerVersionMarkerFileName),
	}
}

// DefaultVMPrivateKeyPath returns the runtime-managed key path used in
// deployment connection metadata.
func DefaultVMPrivateKeyPath(deployment config.DeploymentDir) string {
	return newVMRuntimePaths(deployment).PrivateKeyPath
}

func SharedDir(deployment config.DeploymentDir) string {
	return newVMRuntimePaths(deployment).SharedDir
}
