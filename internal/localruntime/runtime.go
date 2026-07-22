// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blang/semver/v4"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/util"
	"golang.org/x/crypto/ssh"
)

const (
	DirName            = "local"
	runtimeDirName     = "runtime"
	vmDirName          = "vm"
	vmStateFileName    = "vm-state.json"
	vmPIDFileName      = "vm.pid"
	PrivateKeyFileName = "node_access.pem"
	// runnerVersionMarkerFileName records the semver of the runner this
	// deployment was last prepared/started with. It's a launcher-owned file,
	// distinct from vm-state.json, whose schema is dictated by the runner's
	// own external contract and isn't ours to extend.
	runnerVersionMarkerFileName = "runner-version.json"
	openSSHKeyPEMType           = "OPENSSH PRIVATE KEY"
	stopPollInterval            = 500 * time.Millisecond
	stopTimeout                 = 90 * time.Second
	dirMode                     = 0o750
	privateFileMode             = 0o600
	markerFileMode              = 0o600
	executableFileMode          = 0o700
	maxTCPPort                  = 65535
	// Internal escape hatch for development with runners that predate version reporting.
	forceRunnerReconciliationEnv = "EXASOL_LOCAL_FORCE_RUNNER_RECONCILIATION"
	// Internal escape hatch for tests: exasol-local-runner is embed-only, so
	// non-macOS test hosts have no way to resolve a runner through the
	// Manager at all. Setting this to a path bypasses resolution entirely.
	runnerOverridePathEnv       = "EXASOL_LOCAL_RUNNER_OVERRIDE_PATH"
	exasolLocalRunnerResourceID = "exasol-local-runner"
)

type Config struct {
	CPUCount   int
	MemoryMB   int
	DataSizeGB int
	Ports      string
}

type State struct {
	VMIP                   string
	SSHPort                int
	DBPort                 int
	UIPort                 int
	PrivateKeyPath         string
	PrivateKeyRelativePath string
}

type Paths struct {
	Root                    string
	WorkDir                 string
	VMDir                   string
	StatePath               string
	PrivateKeyPath          string
	RunnerVersionMarkerPath string
}

func NewPaths(deployment config.DeploymentDir) Paths {
	root := deployment.Resolve(DirName)
	workDir := filepath.Join(root, runtimeDirName)

	return Paths{
		Root:                    root,
		WorkDir:                 workDir,
		VMDir:                   filepath.Join(workDir, vmDirName),
		StatePath:               filepath.Join(workDir, vmStateFileName),
		PrivateKeyPath:          filepath.Join(root, PrivateKeyFileName),
		RunnerVersionMarkerPath: filepath.Join(workDir, runnerVersionMarkerFileName),
	}
}

func (paths Paths) PrivateKeyRelativePath(deployment config.DeploymentDir) (string, error) {
	rel, err := filepath.Rel(deployment.Root(), paths.PrivateKeyPath)
	if err != nil {
		return "", err
	}

	return filepath.ToSlash(rel), nil
}

type runnerPorts struct {
	SSH int `json:"ssh"`
	DB  int `json:"db"`
	UI  int `json:"ui"`
}

//nolint:tagliatelle // Runner state JSON keys are defined by the runner contract.
type runnerState struct {
	VMName    string      `json:"vm_name"`
	VMIP      string      `json:"vm_ip"`
	CPUCount  string      `json:"cpu_count"`
	RAMSize   string      `json:"ram_size"`
	PID       string      `json:"pid"`
	SharedDir string      `json:"shared_dir"`
	Ports     runnerPorts `json:"ports"`
}

type Runtime struct {
	deployment config.DeploymentDir
	paths      Paths
	manager    *runtimeartifacts.Manager
}

// manager may be nil for operations that never need to invoke the runner
// (e.g. Destroy on a deployment that was never prepared).
func New(deployment config.DeploymentDir, manager *runtimeartifacts.Manager) *Runtime {
	return &Runtime{
		deployment: deployment,
		paths:      NewPaths(deployment),
		manager:    manager,
	}
}

func (runtime *Runtime) Deployment() config.DeploymentDir {
	return runtime.deployment
}

func (runtime *Runtime) Paths() Paths {
	return runtime.paths
}

// VM sizing (CPU/memory/data disk) is not a Prepare concern: it's passed
// directly as RunCommand args for "start".
func (runtime *Runtime) Prepare(ctx context.Context, out, outErr io.Writer) error {
	if err := os.MkdirAll(runtime.paths.WorkDir, dirMode); err != nil {
		return fmt.Errorf("failed to create local runtime directory: %w", err)
	}
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}
	if err := runtime.reconcileRunnerVersion(ctx, runnerPath); err != nil {
		return err
	}
	if err := runtime.ensureSSHKey(); err != nil {
		return err
	}

	return runtime.initializeVMIfNeeded(ctx, runnerPath, out, outErr)
}

func (runtime *Runtime) RunCommand(
	ctx context.Context,
	args []string,
	out, outErr io.Writer,
) error {
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}

	return runtime.runnerCommand(ctx, runnerPath, args, out, outErr)
}

func (runtime *Runtime) ReadState() (*State, error) {
	state, err := readRunnerState(runtime.paths.StatePath)
	if err != nil {
		return nil, err
	}

	return runtime.toState(state)
}

type VMStatus struct {
	Running bool `json:"running"`
}

func (runtime *Runtime) Status(ctx context.Context) (*VMStatus, error) {
	return runnerCommandJSON[VMStatus](ctx, runtime, "status")
}

// PortState is one of the runner's classified per-port reachability states,
// treated as an opaque external contract: this package does not interpret
// how the runner arrived at it, only which value it reported.
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

// HealthCheck asks the runner to freshly probe every forwarded port's guest
// reachability. Unlike Status, this can trigger real network dials on the
// runner side, so callers should only invoke it when they actually need a
// reachability diagnosis, not from routine/frequent code paths.
func (runtime *Runtime) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	return runnerCommandJSON[HealthCheckResult](ctx, runtime, "health-check")
}

// runnerCommandJSON is shared by Status and HealthCheck, which differ only
// in the subcommand name and result shape.
func runnerCommandJSON[T any](
	ctx context.Context,
	runtime *Runtime,
	command string,
) (*T, error) {
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return nil, err
	}

	stdout, err := runtime.runnerCommandWithOutput(ctx, runnerPath, []string{command})
	if err != nil {
		return nil, err
	}

	var result T
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return nil, fmt.Errorf("failed to parse local runner %s output: %w", command, err)
	}

	return &result, nil
}

func (runtime *Runtime) Stop(ctx context.Context, out, outErr io.Writer) error {
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}

	if err := runtime.runnerCommand(ctx, runnerPath, []string{"stop"}, out, outErr); err != nil {
		return err
	}

	return runtime.waitForDaemonExit(ctx)
}

func (runtime *Runtime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	if err := runtime.stopBeforeDestroy(ctx, out, outErr); err != nil {
		return err
	}

	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

// stopBeforeDestroy stops a still-running VM before its files are removed. A
// deployment that was never prepared (paths.VMDir absent) has nothing to
// stop; a runner that fails to resolve or stop is treated the same way,
// since Destroy's job is cleanup, not reporting a runner-invocation failure.
func (runtime *Runtime) stopBeforeDestroy(ctx context.Context, out, outErr io.Writer) error {
	if _, err := os.Stat(runtime.paths.VMDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to inspect local VM directory: %w", err)
	}

	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		//nolint:nilerr // best-effort: cleanup proceeds even if the runner can't be resolved.
		return nil
	}
	if err := runtime.runnerCommand(ctx, runnerPath, []string{"stop"}, out, outErr); err != nil {
		//nolint:nilerr // best-effort: cleanup proceeds even if the running VM can't be stopped.
		return nil
	}

	return runtime.waitForDaemonExit(ctx)
}

// The runner is never copied into the deployment directory: cmd.Dir is set
// to the deployment's working directory independently of wherever the
// manager resolves the binary itself (see runnerCommand/runnerCommandWithOutput),
// so there's nothing for a per-deployment copy to do.
func (runtime *Runtime) resolveRunnerPath(ctx context.Context) (string, error) {
	if override := strings.TrimSpace(os.Getenv(runnerOverridePathEnv)); override != "" {
		return override, nil
	}

	return runtime.manager.Request(ctx, exasolLocalRunnerResourceID)
}

func (runtime *Runtime) initializeVMIfNeeded(
	ctx context.Context,
	runnerPath string,
	out, outErr io.Writer,
) error {
	if _, err := os.Stat(runtime.paths.VMDir); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local VM directory: %w", err)
	}

	return runtime.runnerCommand(
		ctx,
		runnerPath,
		[]string{"init", "--ssh-key", runtime.paths.PrivateKeyPath},
		out,
		outErr,
	)
}

func (runtime *Runtime) toState(state *runnerState) (*State, error) {
	keyFile, err := runtime.paths.PrivateKeyRelativePath(runtime.deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local SSH key path: %w", err)
	}

	return &State{
		VMIP:                   state.VMIP,
		SSHPort:                state.Ports.SSH,
		DBPort:                 state.Ports.DB,
		UIPort:                 state.Ports.UI,
		PrivateKeyPath:         runtime.paths.PrivateKeyPath,
		PrivateKeyRelativePath: keyFile,
	}, nil
}

// reconcileRunnerVersion records/updates this deployment's persisted runner
// version marker based on the runner the manager just resolved. The runner
// is always resolved fresh from the shared cache, so there is no older
// installed copy to fall back to: an unsafe version relationship (a major
// mismatch, or the resolved runner being older than the marker) is logged
// as a warning and the marker is updated to match, rather than refusing to
// proceed.
func (runtime *Runtime) reconcileRunnerVersion(ctx context.Context, runnerPath string) error {
	resolvedVersion, err := readRunnerVersion(ctx, runnerPath)
	forceReconciliation := strings.TrimSpace(os.Getenv(forceRunnerReconciliationEnv)) == "1"
	if err != nil {
		if forceReconciliation {
			slog.Warn(
				"forced local runner reconciliation without version compatibility checks",
				"environmentVariable", forceRunnerReconciliationEnv,
				"versionError", err,
			)

			return nil
		}

		return fmt.Errorf("resolved local runner does not report a valid version: %w", err)
	}

	markerVersion, err := readRunnerVersionMarker(runtime.paths.RunnerVersionMarkerPath)
	if err != nil {
		return writeRunnerVersionMarker(runtime.paths.RunnerVersionMarkerPath, resolvedVersion)
	}

	switch {
	case markerVersion.Major != resolvedVersion.Major:
		slog.Warn(
			"resolved local runner major version differs from this deployment's recorded "+
				"version; proceeding anyway",
			"recordedVersion", markerVersion,
			"resolvedVersion", resolvedVersion,
		)
	case resolvedVersion.LT(markerVersion):
		slog.Warn(
			"resolved local runner is older than this deployment's recorded version; "+
				"proceeding anyway",
			"recordedVersion", markerVersion,
			"resolvedVersion", resolvedVersion,
		)
	case resolvedVersion.GT(markerVersion):
		slog.Info(
			"resolved local runner is newer than this deployment's recorded version",
			"recordedVersion", markerVersion,
			"resolvedVersion", resolvedVersion,
		)
	default:
		// Identical version: nothing to log.
	}

	return writeRunnerVersionMarker(runtime.paths.RunnerVersionMarkerPath, resolvedVersion)
}

func readRunnerVersion(ctx context.Context, runnerPath string) (semver.Version, error) {
	cmd := exec.CommandContext(ctx, runnerPath, "version")
	cmd.Dir = filepath.Dir(runnerPath)
	output, err := cmd.Output()
	if err != nil {
		return semver.Version{}, fmt.Errorf("runner version command failed: %w", err)
	}

	version, err := semver.ParseTolerant(strings.TrimSpace(string(output)))
	if err != nil {
		return semver.Version{}, fmt.Errorf(
			"invalid runner version %q: %w",
			strings.TrimSpace(string(output)),
			err,
		)
	}

	return version, nil
}

//nolint:tagliatelle // Marker file schema is ours; keeping it a single lowercase field.
type runnerVersionMarker struct {
	Version string `json:"version"`
}

func readRunnerVersionMarker(path string) (semver.Version, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return semver.Version{}, err
	}

	var marker runnerVersionMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return semver.Version{}, fmt.Errorf("invalid local runner version marker: %w", err)
	}

	return semver.ParseTolerant(strings.TrimSpace(marker.Version))
}

func writeRunnerVersionMarker(path string, version semver.Version) error {
	data, err := json.Marshal(runnerVersionMarker{Version: version.String()})
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, markerFileMode)
}

// runnerCommandWithOutput runs the runner and returns captured stdout.
// Use this for commands whose output must be parsed (e.g. status JSON).
func (runtime *Runtime) runnerCommandWithOutput(
	ctx context.Context,
	runnerPath string,
	args []string,
) (string, error) {
	return runtime.runRunnerCommand(ctx, runnerPath, args, nil, nil)
}

func (runtime *Runtime) runnerCommand(
	ctx context.Context,
	runnerPath string,
	args []string,
	out, outErr io.Writer,
) error {
	_, err := runtime.runRunnerCommand(ctx, runnerPath, args, out, outErr)

	return err
}

// runRunnerCommand runs the runner and returns captured stdout, additionally
// forwarding stdout/stderr to out/outErr as they arrive (nil is a no-op, see
// util.CombineWriters).
func (runtime *Runtime) runRunnerCommand(
	ctx context.Context,
	runnerPath string,
	args []string,
	out, outErr io.Writer,
) (string, error) {
	if len(args) == 0 {
		return "", errors.New("local runner command is empty")
	}

	cmd := exec.CommandContext(ctx, runnerPath, args...)
	cmd.Dir = runtime.paths.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = util.CombineWriters(&stdout, out)
	cmd.Stderr = util.CombineWriters(&stderr, outErr)

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if detail != "" {
			return "", fmt.Errorf("local runner command %q failed: %w\n%s", args[0], err, detail)
		}

		return "", fmt.Errorf("local runner command %q failed: %w", args[0], err)
	}

	return stdout.String(), nil
}

func (runtime *Runtime) waitForDaemonExit(ctx context.Context) error {
	pid, err := runtime.readDaemonPID()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}
	if !processRunning(pid) {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()

	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf(
				"local runner daemon did not exit within %s after stop signal",
				stopTimeout,
			)
		case <-ticker.C:
			if !processRunning(pid) {
				return nil
			}
		}
	}
}

func (runtime *Runtime) readDaemonPID() (int, error) {
	pidPath := filepath.Join(runtime.paths.WorkDir, vmPIDFileName)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return 0, fmt.Errorf("failed to parse local runner daemon PID from %s: %w", pidPath, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("local runner daemon PID must be greater than zero: %d", pid)
	}

	return pid, nil
}

func processRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}

	return false
}

func (runtime *Runtime) ensureSSHKey() error {
	if keyData, err := os.ReadFile(runtime.paths.PrivateKeyPath); err == nil {
		if isOpenSSHPrivateKey(keyData) {
			return nil
		}

		if err := os.Remove(runtime.paths.PrivateKeyPath); err != nil {
			return fmt.Errorf("failed to replace invalid local SSH key: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local SSH key: %w", err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate local SSH key: %w", err)
	}
	privateKeyBlock, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return fmt.Errorf("failed to marshal local SSH private key: %w", err)
	}
	privateKeyPEM := pem.EncodeToMemory(privateKeyBlock)
	if err := os.MkdirAll(filepath.Dir(runtime.paths.PrivateKeyPath), dirMode); err != nil {
		return fmt.Errorf("failed to create local SSH key directory: %w", err)
	}
	if err := os.WriteFile(
		runtime.paths.PrivateKeyPath,
		privateKeyPEM,
		privateFileMode,
	); err != nil {
		return fmt.Errorf("failed to write local SSH private key: %w", err)
	}

	return nil
}

func isOpenSSHPrivateKey(keyData []byte) bool {
	block, _ := pem.Decode(keyData)
	if block == nil || block.Type != openSSHKeyPEMType {
		return false
	}
	if _, err := ssh.ParsePrivateKey(keyData); err != nil {
		return false
	}

	return true
}

func readRunnerState(statePath string) (*runnerState, error) {
	stateFile, err := os.Open(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local VM state file %s: %w", statePath, err)
	}
	defer stateFile.Close()

	var state runnerState
	if err := json.NewDecoder(stateFile).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to parse local VM state file %s: %w", statePath, err)
	}
	if err := validateRunnerState(&state); err != nil {
		return nil, err
	}

	return &state, nil
}

func validateRunnerState(state *runnerState) error {
	if state == nil {
		return errors.New("local VM state is missing")
	}
	if err := validatePort("ssh", state.Ports.SSH); err != nil {
		return err
	}
	if err := validatePort("database", state.Ports.DB); err != nil {
		return err
	}
	if state.Ports.UI != 0 {
		return validatePort("ui", state.Ports.UI)
	}

	return nil
}

func validatePort(name string, port int) error {
	if port <= 0 || port > maxTCPPort {
		return fmt.Errorf("local VM state contains invalid %s port: %d", name, port)
	}

	return nil
}
