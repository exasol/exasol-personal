// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

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

// HostPlatform identifies which direct-host platform a HostRuntime serves.
// Callers use it to select platform-specific user guidance without needing a
// distinct runtime type per platform.
type HostPlatform string

const (
	HostPlatformLinux   HostPlatform = "linux"
	HostPlatformWindows HostPlatform = "windows"
)

// HostChangeKind identifies a host environment change that requires explicit
// user approval before a runtime can apply it.
type HostChangeKind string

// HostChangeInstallContainerRuntime covers installing the container runtime
// (Podman) onto the host, which mutates state shared beyond this deployment.
const HostChangeInstallContainerRuntime HostChangeKind = "install-container-runtime"

// HostCommand describes a command that preparation intends to run. It is
// data, not a closure, so the approver can show the user exactly what will
// execute before deciding.
type HostCommand struct {
	Name string
	Args []string
}

// String renders the command as a single shell-style line for display.
func (command HostCommand) String() string {
	return strings.TrimSpace(strings.Join(append([]string{command.Name}, command.Args...), " "))
}

// HostChangeRequest describes an approval-gated host environment change.
type HostChangeRequest struct {
	Kind     HostChangeKind
	Commands []HostCommand
}

// HostChangeApprover decides whether an approval-gated host change may run.
// Returning false (rather than an error) means the user declined; the runtime
// turns that into a failure explaining how to proceed.
type HostChangeApprover func(context.Context, HostChangeRequest) (bool, error)

// PrepareOptions carries preparation policy supplied by the command layer.
// Keeping approval and progress presentation here means a runtime never has
// to know whether it is attached to a terminal.
type PrepareOptions struct {
	// ApproveHostChange gates every host-mutating step. A nil approver
	// denies all of them, so a caller that forgets to supply one fails
	// safe instead of silently mutating the host.
	ApproveHostChange HostChangeApprover
	// Progress receives human-readable preparation progress. Unlike the
	// --verbose-gated subprocess output, this is always shown: it carries
	// multi-minute steps the user needs to see.
	Progress io.Writer
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
	Prepare(ctx context.Context, out, outErr io.Writer, options PrepareOptions) error
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
