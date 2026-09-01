// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/resource"
	"github.com/exasol/exasol-personal/internal/util"
)

const (
	vmPIDFileName      = "vm.pid"
	stopPollInterval   = 500 * time.Millisecond
	stopTimeout        = 90 * time.Second
	dirMode            = 0o750
	markerFileMode     = 0o600
	executableFileMode = 0o700
	artifactFileMode   = 0o640
	maxTCPPort         = 65535
	minimumRunnerMajor = 2

	vmNanoDataDir          = "/var/lib/exa"
	nanoArtifactDirName    = "resources"
	nanoArtifactFileName   = "nano.tar"
	legacyNanoContainer    = "exasol-local-db"
	forwardDatabaseService = "db"

	// Internal escape hatch for development with runners that do not report semver.
	forceRunnerReconciliationEnv = "EXASOL_LOCAL_FORCE_RUNNER_RECONCILIATION"
	// Internal escape hatch for local runner integration tests and development.
	runnerOverridePathEnv       = "EXASOL_LOCAL_RUNNER_OVERRIDE_PATH"
	exasolLocalRunnerResourceID = "exasol-local-runner"
)

const containerShellScript = `set -eu
container_name=$1
if ! podman container exists "$container_name"; then
    printf 'Exasol Local database container not found\n' >&2
    podman ps -a >&2
    exit 125
fi
rootfs=$(podman mount "$container_name")
pid=$(podman inspect "$container_name" --format '{{.State.Pid}}')
printf 'Nano does not include a shell; using the VM shell from the container rootfs.\n'
printf 'Container rootfs: %s\n' "$rootfs"
cd "$rootfs"
if [ -n "$pid" ] && [ "$pid" != 0 ]; then
    exec nsenter --target "$pid" --uts --ipc --net /bin/sh
fi
exec /bin/sh`

//nolint:tagliatelle // Runner state JSON keys are defined by the runner contract.
type runnerForwardState struct {
	GuestPort int `json:"guest_port"`
	HostPort  int `json:"host_port"`
}

//nolint:tagliatelle // Runner state JSON keys are defined by the runner contract.
type runnerState struct {
	VMName    string                        `json:"vm_name"`
	CPUCount  string                        `json:"cpu_count"`
	RAMSize   string                        `json:"ram_size"`
	PID       string                        `json:"pid"`
	SharedDir string                        `json:"shared_dir"`
	Forwards  map[string]runnerForwardState `json:"forwards"`
}

type MacVMRuntime struct {
	deployment     config.DeploymentDir
	paths          vmRuntimePaths
	resolver       *resource.Resolver
	endpoint       *RuntimeEndpoint
	installFactory func(string) (localinstall.LocalInstall, error)
}

// NewMacVMRuntime: a nil resolver is valid until an operation invokes the runner.
func NewMacVMRuntime(
	deployment config.DeploymentDir,
	resolver *resource.Resolver,
) *MacVMRuntime {
	return &MacVMRuntime{
		deployment: deployment,
		paths:      newVMRuntimePaths(deployment),
		resolver:   resolver,
	}
}

func (runtime *MacVMRuntime) Deployment() config.DeploymentDir {
	return runtime.deployment
}

// Prepare initializes the local runtime without starting the VM.
//
// The macOS runtime needs no approval-gated host changes: Podman ships
// inside the managed VM, so nothing outside the deployment is touched.
func (runtime *MacVMRuntime) Prepare(
	ctx context.Context, out, outErr io.Writer, _ PrepareOptions,
) error {
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

	return runtime.initializeVMIfNeeded(ctx, runnerPath, out, outErr)
}

func (runtime *MacVMRuntime) Start(
	ctx context.Context,
	out, outErr io.Writer,
	runtimeConfig VMConfig,
) error {
	hostDBPort, err := resolveMacHostDBPort(runtimeConfig.Ports)
	if err != nil {
		return err
	}
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}
	if err := runtime.reconcileRunnerVersion(ctx, runnerPath); err != nil {
		return err
	}

	args := []string{
		"start",
		"--forward", fmt.Sprintf("%s:%d:%d", forwardDatabaseService, nanoDBPort, hostDBPort),
		strconv.Itoa(runtimeConfig.CPUCount),
		strconv.Itoa(runtimeConfig.MemoryMB),
		strconv.Itoa(runtimeConfig.DataSizeGB),
	}
	if err := runtime.runnerCommand(ctx, runnerPath, args, out, outErr); err != nil {
		return err
	}

	state, err := readRunnerState(runtime.paths.StatePath)
	if err != nil {
		return runtime.stopAfterStartFailure(ctx, runnerPath, out, outErr, err)
	}
	endpoint, err := runtimeEndpointFromRunnerState(state)
	if err != nil {
		return runtime.stopAfterStartFailure(ctx, runnerPath, out, outErr, err)
	}
	install, err := runtime.install(runnerPath)
	if err != nil {
		return runtime.stopAfterStartFailure(ctx, runnerPath, out, outErr, err)
	}
	startConfig := localinstall.StartConfig{
		ContainerDBPort:      nanoDBPort,
		DataDir:              vmNanoDataDir,
		InitParams:           append([]string(nil), nanoInitParams...),
		VersionCheck:         runtimeConfig.VersionCheck,
		SLCs:                 runtimeConfig.SLCs,
		LegacyContainerNames: []string{legacyNanoContainer},
	}
	if err := install.Start(ctx, out, outErr, startConfig); err != nil {
		return runtime.stopAfterStartFailure(ctx, runnerPath, out, outErr, err)
	}

	runtime.endpoint = endpoint

	return nil
}

func (runtime *MacVMRuntime) ReadEndpoints() (*VMRuntimeEndpoint, error) {
	if runtime.endpoint != nil {
		return &VMRuntimeEndpoint{RuntimeEndpoint: *runtime.endpoint}, nil
	}
	state, err := readRunnerState(runtime.paths.StatePath)
	if err != nil {
		return nil, err
	}
	endpoint, err := runtimeEndpointFromRunnerState(state)
	if err != nil {
		return nil, err
	}
	runtime.endpoint = endpoint

	return &VMRuntimeEndpoint{RuntimeEndpoint: *endpoint}, nil
}

// EnsureQueryable is a no-op for the VM runtime. Its runner is a host-side
// binary that reports VM state whether or not the VM is running, so Status
// can always answer and nothing has to be started to observe it.
func (*MacVMRuntime) EnsureQueryable(context.Context, io.Writer, io.Writer) error {
	return nil
}

func (runtime *MacVMRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	vmStatus, err := runnerCommandJSON[RuntimeStatus](ctx, runtime, "status")
	if err != nil || !vmStatus.Running {
		return vmStatus, err
	}
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return nil, err
	}
	install, err := runtime.install(runnerPath)
	if err != nil {
		return nil, err
	}
	installStatus, err := install.Status(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect Nano in the local VM: %w", err)
	}

	return &RuntimeStatus{Running: installStatus.Running}, nil
}

func (runtime *MacVMRuntime) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	return runnerCommandJSON[HealthCheckResult](ctx, runtime, "health-check")
}

func (runtime *MacVMRuntime) OpenHostShell(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}

	return runtime.runInteractiveRunnerCommand(
		ctx, runnerPath, []string{"run"}, stdin, stdout, stderr,
	)
}

func (runtime *MacVMRuntime) OpenContainerShell(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	containerName, err := localinstall.ContainerName(runtime.deployment)
	if err != nil {
		return err
	}
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}

	return runtime.runInteractiveRunnerCommand(
		ctx,
		runnerPath,
		[]string{
			"run", "--tty", "--", "sh", "-c", containerShellScript, "sh", containerName,
		},
		stdin,
		stdout,
		stderr,
	)
}

func runnerCommandJSON[T any](
	ctx context.Context,
	runtime *MacVMRuntime,
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

func (runtime *MacVMRuntime) Stop(ctx context.Context, out, outErr io.Writer) error {
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}
	vmStatus, err := runnerCommandJSON[RuntimeStatus](ctx, runtime, "status")
	if err != nil {
		return err
	}
	if vmStatus.Running {
		install, installErr := runtime.install(runnerPath)
		if installErr != nil {
			return installErr
		}
		if installErr := install.Stop(ctx, out, outErr); installErr != nil {
			return installErr
		}
	}
	runtime.endpoint = nil

	return runtime.stopVM(ctx, runnerPath, out, outErr)
}

func (runtime *MacVMRuntime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	if err := runtime.cleanupBeforeDestroy(ctx, out, outErr); err != nil {
		return err
	}
	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

func (runtime *MacVMRuntime) WorkaroundNanoStartupDurability(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}
	environment := newRunnerExecutionEnvironment(runnerPath, runtime.paths.WorkDir)
	if err := environment.Sync(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to apply Nano startup durability workaround: %w", err)
	}

	return nil
}

func (runtime *MacVMRuntime) stopAfterStartFailure(
	ctx context.Context,
	runnerPath string,
	out, outErr io.Writer,
	startErr error,
) error {
	stopErr := runtime.stopVM(ctx, runnerPath, out, outErr)
	if stopErr != nil {
		return errors.Join(
			startErr,
			fmt.Errorf("failed to stop VM after startup failure: %w", stopErr),
		)
	}

	return startErr
}

func (runtime *MacVMRuntime) stopVM(
	ctx context.Context,
	runnerPath string,
	out, outErr io.Writer,
) error {
	if err := runtime.runnerCommand(
		ctx, runnerPath, []string{"stop"}, out, outErr,
	); err != nil {
		return err
	}

	return runtime.waitForDaemonExit(ctx)
}

func (runtime *MacVMRuntime) cleanupBeforeDestroy(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	if _, err := os.Stat(runtime.paths.VMDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to inspect local VM directory: %w", err)
	}
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return err
	}
	vmStatus, err := runnerCommandJSON[RuntimeStatus](ctx, runtime, "status")
	if err != nil {
		return err
	}
	if !vmStatus.Running {
		return nil
	}
	install, err := runtime.install(runnerPath)
	if err != nil {
		return err
	}
	if err := install.Destroy(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to remove Nano before destroying the VM: %w", err)
	}

	return runtime.stopVM(ctx, runnerPath, out, outErr)
}

func (runtime *MacVMRuntime) install(
	runnerPath string,
) (localinstall.LocalInstall, error) {
	if runtime.installFactory != nil {
		return runtime.installFactory(runnerPath)
	}
	mapper := newVMPathMapper(runtime.paths.SharedDir)
	slcStagingDir, err := mapper.Map(SLCStagingDir(runtime.deployment))
	if err != nil {
		return nil, err
	}
	slcStatusPath, err := mapper.Map(SLCStatusPath(runtime.deployment))
	if err != nil {
		return nil, err
	}
	resolveImage := func(ctx context.Context) (localinstall.RuntimePath, error) {
		sourcePath, err := localinstall.ResolveNanoImage(ctx, runtime.resolver)
		if err != nil {
			return localinstall.RuntimePath{}, err
		}
		hostPath := filepath.Join(
			runtime.paths.SharedDir,
			nanoArtifactDirName,
			nanoArtifactFileName,
		)
		if err := materializeFileAtomically(sourcePath, hostPath); err != nil {
			return localinstall.RuntimePath{}, fmt.Errorf(
				"failed to stage Nano image for VM: %w", err,
			)
		}

		return mapper.Map(hostPath)
	}

	return localinstall.NewPodmanInstallWithEnvironment(
		runtime.deployment,
		newRunnerExecutionEnvironment(runnerPath, runtime.paths.WorkDir),
		resolveImage,
		slcStagingDir,
		slcStatusPath,
	), nil
}

func materializeFileAtomically(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath) //nolint:gosec // resource path
	if err != nil {
		return fmt.Errorf("failed to open source artifact %s: %w", sourcePath, err)
	}
	defer func() { _ = source.Close() }()
	sourceInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("failed to inspect source artifact %s: %w", sourcePath, err)
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source artifact is a directory: %s", sourcePath)
	}
	if targetInfo, statErr := os.Stat(targetPath); statErr == nil &&
		targetInfo.Size() == sourceInfo.Size() &&
		targetInfo.ModTime().Equal(sourceInfo.ModTime()) &&
		targetInfo.Mode().Perm() == artifactFileMode {
		return nil
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect staged artifact %s: %w", targetPath, statErr)
	}

	directory := filepath.Dir(targetPath)
	if err := os.MkdirAll(directory, dirMode); err != nil {
		return fmt.Errorf("failed to create resource directory %s: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, ".nano-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create staged artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if _, err := io.Copy(temporary, source); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("failed to copy Nano image: %w", err)
	}
	if err := temporary.Chmod(artifactFileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("failed to set staged Nano image permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("failed to sync staged Nano image: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close staged Nano image: %w", err)
	}
	if err := os.Chtimes(temporaryPath, sourceInfo.ModTime(), sourceInfo.ModTime()); err != nil {
		return fmt.Errorf("failed to preserve staged Nano image timestamp: %w", err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("failed to replace staged Nano image %s: %w", targetPath, err)
	}

	return nil
}

func resolveMacHostDBPort(ports string) (int, error) {
	ports = strings.TrimSpace(ports)
	if ports == "" {
		return nanoDBPort, nil
	}
	if ports == "auto" {
		return 0, nil
	}

	hostDBPort := nanoDBPort
	dbPortConfigured := false
	for rawEntry := range strings.SplitSeq(ports, ",") {
		entry := strings.TrimSpace(rawEntry)
		service, rawPort, found := strings.Cut(entry, ":")
		service = strings.TrimSpace(service)
		rawPort = strings.TrimSpace(rawPort)
		if !found || service == "" || rawPort == "" {
			return 0, fmt.Errorf("invalid local port mapping %q; expected <service>:<port>", entry)
		}
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 0 || port > maxTCPPort {
			return 0, fmt.Errorf("invalid local port %q for service %q", rawPort, service)
		}
		if service != forwardDatabaseService {
			continue
		}
		if dbPortConfigured {
			return 0, errors.New("local database port is configured more than once")
		}
		hostDBPort = port
		dbPortConfigured = true
	}

	return hostDBPort, nil
}

func runtimeEndpointFromRunnerState(state *runnerState) (*RuntimeEndpoint, error) {
	if state == nil {
		return nil, errors.New("local VM state is missing")
	}
	forward, exists := state.Forwards[forwardDatabaseService]
	if !exists {
		return nil, errors.New("local VM state is missing the database forward")
	}
	if forward.GuestPort != nanoDBPort {
		return nil, fmt.Errorf(
			"local VM state contains database guest port %d, expected %d",
			forward.GuestPort,
			nanoDBPort,
		)
	}
	if err := validatePort("database", forward.HostPort); err != nil {
		return nil, err
	}

	return &RuntimeEndpoint{DBPort: forward.HostPort, ShellSupported: true}, nil
}

func (runtime *MacVMRuntime) resolveRunnerPath(ctx context.Context) (string, error) {
	if override := strings.TrimSpace(os.Getenv(runnerOverridePathEnv)); override != "" {
		return override, nil
	}
	if runtime.resolver == nil {
		return "", errors.New("local runner artifact resolver is required")
	}

	return runtime.resolver.Resolve(ctx, exasolLocalRunnerResourceID)
}

func (runtime *MacVMRuntime) initializeVMIfNeeded(
	ctx context.Context,
	runnerPath string,
	out, outErr io.Writer,
) error {
	if _, err := os.Stat(runtime.paths.VMDir); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local VM directory: %w", err)
	}

	return runtime.runnerCommand(ctx, runnerPath, []string{"init"}, out, outErr)
}

// reconcileRunnerVersion enforces the VM-only v2 contract and records the
// resolved runner version for deployment diagnostics.
func (runtime *MacVMRuntime) reconcileRunnerVersion(ctx context.Context, runnerPath string) error {
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
	if resolvedVersion.Major < minimumRunnerMajor {
		return fmt.Errorf(
			"resolved local runner %s uses the legacy application-owning contract; "+
				"version 2 or newer is required",
			resolvedVersion,
		)
	}

	markerVersion, err := readRunnerVersionMarker(runtime.paths.RunnerVersionMarkerPath)
	if err != nil {
		return writeRunnerVersionMarker(runtime.paths.RunnerVersionMarkerPath, resolvedVersion)
	}

	switch {
	case markerVersion.Major != resolvedVersion.Major:
		slog.Warn(
			"resolved local runner major version differs from this deployment's recorded version",
			"recordedVersion", markerVersion,
			"resolvedVersion", resolvedVersion,
		)
	case resolvedVersion.LT(markerVersion):
		slog.Warn(
			"resolved local runner is older than this deployment's recorded version",
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
		// Identical versions require no diagnostic message.
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

func (runtime *MacVMRuntime) runnerCommandWithOutput(
	ctx context.Context,
	runnerPath string,
	args []string,
) (string, error) {
	return runtime.runRunnerCommand(ctx, runnerPath, args, nil, nil)
}

func (runtime *MacVMRuntime) runnerCommand(
	ctx context.Context,
	runnerPath string,
	args []string,
	out, outErr io.Writer,
) error {
	_, err := runtime.runRunnerCommand(ctx, runnerPath, args, out, outErr)

	return err
}

func (runtime *MacVMRuntime) runRunnerCommand(
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

func (runtime *MacVMRuntime) runInteractiveRunnerCommand(
	ctx context.Context,
	runnerPath string,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	cmd := exec.CommandContext(ctx, runnerPath, args...)
	cmd.Dir = runtime.paths.WorkDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local runner command %q failed: %w", args[0], err)
	}

	return nil
}

func (runtime *MacVMRuntime) waitForDaemonExit(ctx context.Context) error {
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

func (runtime *MacVMRuntime) readDaemonPID() (int, error) {
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

	return err == nil || errors.Is(err, syscall.EPERM)
}

func readRunnerState(statePath string) (*runnerState, error) {
	stateFile, err := os.Open(statePath) //nolint:gosec // deployment-owned state path
	if err != nil {
		return nil, fmt.Errorf("failed to open local VM state file %s: %w", statePath, err)
	}
	defer func() { _ = stateFile.Close() }()

	var state runnerState
	if err := json.NewDecoder(stateFile).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to parse local VM state file %s: %w", statePath, err)
	}
	if _, err := runtimeEndpointFromRunnerState(&state); err != nil {
		return nil, err
	}

	return &state, nil
}

func validatePort(name string, port int) error {
	if port <= 0 || port > maxTCPPort {
		return fmt.Errorf("local VM state contains invalid %s port: %d", name, port)
	}

	return nil
}
