// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	nanoDBPort      = 8563
	nanoDataDirName = "exa"
)

var nanoInitParams = []string{"maxConnectionsLicenseLimit=20"}

// hostRuntimeEnvironmentPreparer supplies everything that differs between
// direct-host platforms, so the container lifecycle below stays free of
// platform branching. Each platform contributes three things:
//
//   - EnsureReady: satisfy prerequisites, requesting approval for any step
//     that mutates the host.
//   - EnsureStartable: re-check the cheap, approval-free prerequisites that
//     can lapse between Prepare and Start.
//   - NewExecutionEnvironment: build the environment PodmanInstall runs
//     through, which is where a platform redirects individual commands.
type hostRuntimeEnvironmentPreparer interface {
	Platform() HostPlatform
	EnsureReady(ctx context.Context, options PrepareOptions) error
	EnsureStartable(ctx context.Context, out, outErr io.Writer) error
	NewExecutionEnvironment(runtimeExec []string) localinstall.ExecutionEnvironment
}

// linuxHostEnvironmentPreparer requires nothing but a Podman on PATH: the
// host kernel runs the containers directly, so there is no VM to manage and
// nothing to install on the user's behalf.
type linuxHostEnvironmentPreparer struct{}

func (linuxHostEnvironmentPreparer) Platform() HostPlatform { return HostPlatformLinux }

func (linuxHostEnvironmentPreparer) EnsureReady(_ context.Context, _ PrepareOptions) error {
	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf("'podman' is required to run exasol locally on this platform: %w", err)
	}

	return nil
}

func (linuxHostEnvironmentPreparer) EnsureStartable(
	_ context.Context, _, _ io.Writer,
) error {
	return nil
}

func (linuxHostEnvironmentPreparer) NewExecutionEnvironment(
	runtimeExec []string,
) localinstall.ExecutionEnvironment {
	return localinstall.NewDirectExecutionEnvironment(runtimeExec)
}

// HostRuntime owns direct-host prerequisites and delegates the database
// container lifecycle to localinstall. Platform differences arrive through
// the injected preparer rather than through branches in this type.
type HostRuntime struct {
	deployment  config.DeploymentDir
	paths       runtimePaths
	manager     *runtimeartifacts.Manager
	preparer    hostRuntimeEnvironmentPreparer
	endpoint    *RuntimeEndpoint
	runtimeExec []string
	dialContext func(context.Context, string, string) (net.Conn, error)
}

// NewHostLinuxRuntime creates a Linux host runtime. The manager may be nil for operations
// that never invoke the runner, such as destroying an unprepared deployment.
func NewHostLinuxRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *HostRuntime {
	return newHostRuntime(deployment, manager, linuxHostEnvironmentPreparer{})
}

func newHostRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
	preparer hostRuntimeEnvironmentPreparer,
) *HostRuntime {
	return &HostRuntime{
		deployment: deployment,
		paths:      newRuntimePaths(deployment),
		manager:    manager,
		preparer:   preparer,
	}
}

func (runtime *HostRuntime) Deployment() config.DeploymentDir {
	return runtime.deployment
}

// Platform reports which direct-host platform this runtime serves, so
// callers can select platform-specific user guidance.
func (runtime *HostRuntime) Platform() HostPlatform {
	return runtime.preparer.Platform()
}

// Prepare satisfies the platform's prerequisites and creates the runtime
// work directory. VM sizing (CPU/memory/data disk) is not a Prepare concern:
// it is passed directly as RunCommand args for "start".
//
// Progress and approval come from options, not from out/outErr: preparation
// messaging must stay visible regardless of the --verbose gate that governs
// subprocess output.
func (runtime *HostRuntime) Prepare(
	ctx context.Context,
	_, _ io.Writer,
	options PrepareOptions,
) error {
	if err := runtime.preparer.EnsureReady(ctx, options); err != nil {
		return err
	}
	if err := os.MkdirAll(runtime.paths.WorkDir, dirMode); err != nil {
		return fmt.Errorf("failed to create local runtime directory: %w", err)
	}

	return nil
}

func (runtime *HostRuntime) Start(
	ctx context.Context,
	out, outErr io.Writer,
	runtimeConfig VMConfig,
) error {
	// Re-check prerequisites that can lapse between Prepare and Start (a
	// stopped Podman machine, say). Without this the failure surfaces later
	// as an opaque "podman socket unreachable".
	if err := runtime.preparer.EnsureStartable(ctx, out, outErr); err != nil {
		return err
	}
	startConfig, err := runtime.podmanStartConfig(runtimeConfig.RuntimeConfig)
	if err != nil {
		return err
	}
	if err := runtime.install().Start(ctx, out, outErr, startConfig); err != nil {
		return err
	}
	runtime.endpoint = &RuntimeEndpoint{DBPort: startConfig.ContainerDBPort}

	return nil
}

func (runtime *HostRuntime) Stop(ctx context.Context, out, outErr io.Writer) error {
	return runtime.install().Stop(ctx, out, outErr)
}

func (runtime *HostRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	status, err := runtime.install().Status(ctx, nil, nil)
	if err != nil {
		return nil, err
	}

	return &RuntimeStatus{Running: status.Running}, nil
}

// EnsureQueryable re-runs the platform's approval-free prerequisites so the
// runtime can be observed. On Windows that starts a stopped Podman machine,
// without which Status cannot distinguish a stopped container from a machine
// it simply cannot reach.
func (runtime *HostRuntime) EnsureQueryable(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	return runtime.preparer.EnsureStartable(ctx, out, outErr)
}

func (runtime *HostRuntime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	if err := runtime.install().Destroy(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to remove deployment: %w", err)
	}

	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

func (runtime *HostRuntime) WorkaroundNanoStartupDurability(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	environment := runtime.preparer.NewExecutionEnvironment(runtime.runCmd())
	if err := environment.Sync(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to apply Nano startup durability workaround: %w", err)
	}

	return nil
}

func (runtime *HostRuntime) ReadEndpoints() (*VMRuntimeEndpoint, error) {
	if runtime.endpoint == nil {
		info, err := config.ReadDeploymentInfo(runtime.Deployment())
		if err != nil {
			return nil, fmt.Errorf("host runtime endpoint is unavailable: %w", err)
		}
		if info.Connection == nil || info.Connection.DBPort <= 0 || info.Connection.DBPort > 65535 {
			return nil, errors.New(
				"host runtime deployment information has no valid database endpoint",
			)
		}
		runtime.endpoint = &RuntimeEndpoint{DBPort: info.Connection.DBPort}
	}

	return &VMRuntimeEndpoint{RuntimeEndpoint: *runtime.endpoint}, nil
}

func (runtime *HostRuntime) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	endpoint, err := runtime.ReadEndpoints()
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(endpoint.DBPort))
	dialContext := runtime.dialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{}).DialContext
	}
	connection, err := dialContext(ctx, "tcp", address)
	state := classifyHostPortHealth(err)
	if connection != nil {
		_ = connection.Close()
	}

	return &HealthCheckResult{Ports: map[string]PortHealth{
		"db": {State: state},
	}}, nil
}

func (*HostRuntime) OpenHostShell(
	context.Context,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return ErrHostShellUnsupported
}

func (*HostRuntime) OpenContainerShell(
	context.Context,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return ErrContainerShellUnsupported
}

func classifyHostPortHealth(err error) PortState {
	if err == nil {
		return PortStateReachable
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return PortStateTimeout
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return PortStateTimeout
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return PortStateRefused
	}
	// The error slipped past every well-known classifier. This is where
	// Windows-specific errnos (WSAENOTCONN, WSAECONNABORTED, etc.) end
	// up today; logging the underlying error keeps future occurrences
	// diagnosable from deployment.log alone.
	slog.Debug("classifying host port dial error as blocked",
		"error", err.Error(), "errorType", fmt.Sprintf("%T", err))

	return PortStateBlocked
}

func (runtime *HostRuntime) install() *localinstall.PodmanInstall {
	resolveImage := func(ctx context.Context) (localinstall.RuntimePath, error) {
		path, err := localinstall.ResolveNanoImage(ctx, runtime.manager)
		if err != nil {
			return localinstall.RuntimePath{}, err
		}

		return localinstall.IdentityRuntimePath(path), nil
	}

	return localinstall.NewPodmanInstallWithEnvironment(
		runtime.Deployment(),
		runtime.preparer.NewExecutionEnvironment(runtime.runCmd()),
		resolveImage,
		localinstall.IdentityRuntimePath(SLCStagingDir(runtime.Deployment())),
		localinstall.IdentityRuntimePath(SLCStatusPath(runtime.Deployment())),
	)
}

func (runtime *HostRuntime) runCmd() []string {
	return runtime.runtimeExec
}

func (runtime *HostRuntime) podmanStartConfig(
	runtimeConfig RuntimeConfig,
) (localinstall.StartConfig, error) {
	hostDBPort, err := resolveHostPodmanDBPort(runtimeConfig.Ports)
	if err != nil {
		return localinstall.StartConfig{}, err
	}

	return localinstall.StartConfig{
		ContainerDBPort: hostDBPort,
		DataDir:         filepath.Join(runtime.paths.WorkDir, nanoDataDirName),
		InitParams:      append([]string(nil), nanoInitParams...),
		VersionCheck:    runtimeConfig.VersionCheck,
		SLCs:            runtimeConfig.SLCs,
	}, nil
}

func resolveHostPodmanDBPort(ports string) (int, error) {
	hostDBPort := nanoDBPort
	dbPortConfigured := false
	if strings.TrimSpace(ports) == "" {
		return hostDBPort, nil
	}

	for rawEntry := range strings.SplitSeq(ports, ",") {
		entry := strings.TrimSpace(rawEntry)
		service, rawPort, found := strings.Cut(entry, ":")
		service = strings.TrimSpace(service)
		rawPort = strings.TrimSpace(rawPort)
		if !found || service == "" || rawPort == "" {
			return 0, fmt.Errorf("invalid local port mapping %q; expected <service>:<port>", entry)
		}
		port, err := strconv.Atoi(rawPort)
		if err != nil || port <= 0 || port > 65535 {
			return 0, fmt.Errorf("invalid local port %q for service %q", rawPort, service)
		}
		if service != "db" {
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
