// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

const (
	dirName        = "local"
	runtimeDirName = "runtime"

	vmDirName         = "vm"
	sharedDirName     = "vm-shared"
	vmStateFileName   = "vm-state.json"
	slcStagingDirName = "slc-packages"
	slcStatusFileName = "slc-status.json"
	// runnerVersionMarkerFileName records the semver of the runner this
	// deployment was last prepared/started with. It's a launcher-owned file,
	// distinct from vm-state.json, whose schema is dictated by the runner's
	// own external contract and isn't ours to extend.
	runnerVersionMarkerFileName = "runner-version.json"
)

type RuntimeConfig struct {
	Ports        string
	VersionCheck localinstall.VersionCheckConfig
	SLCs         []localinstall.SLCConfig
}

type VMConfig struct {
	RuntimeConfig

	CPUCount   int
	MemoryMB   int
	DataSizeGB int
}

type RuntimeStatus struct {
	Running bool `json:"running"`
}

// RuntimeEndpoint describes application endpoints and capabilities published
// by a local runtime without exposing its command transport.
type RuntimeEndpoint struct {
	DBPort         int
	UIPort         int
	ShellSupported bool
}

type VMRuntimeEndpoint struct {
	RuntimeEndpoint
}

var (
	ErrHostShellUnsupported      = errors.New("host shell is not supported by this local runtime")
	ErrContainerShellUnsupported = errors.New(
		"container shell is not supported by this local runtime",
	)
)

type PortState string

const (
	PortStateReachable PortState = "reachable"
	PortStateRefused   PortState = "refused"
	PortStateBlocked   PortState = "blocked"
	PortStateTimeout   PortState = "timeout"
)

type PortHealth struct {
	State PortState `json:"state"`
}

type HealthCheckResult struct {
	Ports map[string]PortHealth `json:"ports"`
}

// Generic local runtime interface.
// It intentionally owns the complete lifecycle and durability contract.
// nolint: interfacebloat
type Runtime interface {
	Deployment() config.DeploymentDir
	Prepare(ctx context.Context, in io.Reader, out, outErr io.Writer) error
	Start(ctx context.Context, out, outErr io.Writer, runtimeConfig VMConfig) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	Status(ctx context.Context) (*RuntimeStatus, error)
	Destroy(ctx context.Context, out, outErr io.Writer) error
	// WorkaroundNanoStartupDurability flushes runtime storage after Nano becomes ready.
	// Remove it when Nano durably commits its startup files itself.
	// See SPOT-32205
	WorkaroundNanoStartupDurability(ctx context.Context, out, outErr io.Writer) error

	ReadEndpoints() (*VMRuntimeEndpoint, error)
	HealthCheck(ctx context.Context) (*HealthCheckResult, error)
	OpenHostShell(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error
	OpenContainerShell(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error
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

type vmRuntimePaths struct {
	runtimePaths

	VMDir                   string
	SharedDir               string
	StatePath               string
	RunnerVersionMarkerPath string
}

func newVMRuntimePaths(deployment config.DeploymentDir) vmRuntimePaths {
	rtPaths := newRuntimePaths(deployment)

	return vmRuntimePaths{
		runtimePaths:            rtPaths,
		VMDir:                   filepath.Join(rtPaths.WorkDir, vmDirName),
		SharedDir:               filepath.Join(rtPaths.WorkDir, sharedDirName),
		StatePath:               filepath.Join(rtPaths.WorkDir, vmStateFileName),
		RunnerVersionMarkerPath: filepath.Join(rtPaths.WorkDir, runnerVersionMarkerFileName),
	}
}

func SharedDir(deployment config.DeploymentDir) string {
	return newVMRuntimePaths(deployment).SharedDir
}

func SLCStagingDir(deployment config.DeploymentDir) string {
	return filepath.Join(SharedDir(deployment), slcStagingDirName)
}

func SLCStatusPath(deployment config.DeploymentDir) string {
	return filepath.Join(SharedDir(deployment), slcStatusFileName)
}
