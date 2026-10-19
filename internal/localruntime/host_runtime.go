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
	nanoDBPort       = 8563
	nanoDataDirName  = "exa"
	dataDirMode      = 0o755
	hostLoopbackHost = "127.0.0.1"
)

// userStagingDirs are the data directory paths user documentation directs
// users to write: BucketFS content and JDBC drivers.
var userStagingDirs = []string{
	filepath.Join("bucketfs", "bfsdefault", "default"),
	"jdbc",
}

var nanoInitParams = []string{"maxConnectionsLicenseLimit=20"}

// hostRuntimeEnvironmentPreparer supplies everything that differs between
// direct-host platforms, so the container lifecycle below stays free of
// platform branching. Each platform contributes three things:
//
//   - EnsureReady: satisfy prerequisites, requesting approval for any step
//     that mutates the host.
//   - EnsureStartable: re-check the cheap, approval-free prerequisites that
//     can lapse between Prepare and Start.
//   - ContainerHostRunning: report whether whatever hosts the containers is
//     running, reading state only and never starting anything.
//   - NewExecutionEnvironment: build the environment PodmanInstall runs
//     through, which is where a platform redirects individual commands.
type hostRuntimeEnvironmentPreparer interface {
	Platform() HostPlatform
	EnsureReady(ctx context.Context, options PrepareOptions) error
	EnsureStartable(ctx context.Context, out, outErr io.Writer) error
	ContainerHostRunning(ctx context.Context) (bool, error)
	NewExecutionEnvironment(runtimeExec []string) localinstall.ExecutionEnvironment
}

// hostContainerEngine is the container CLI direct-host runtimes use. It must
// already be on PATH; the launcher never installs it.
const hostContainerEngine = localinstall.EngineDocker

func requireHostContainerEngine() error {
	if _, err := exec.LookPath(string(hostContainerEngine)); err != nil {
		return fmt.Errorf(
			"'%s' is required to run exasol locally on this platform: %w",
			hostContainerEngine, err,
		)
	}

	return nil
}

// linuxHostEnvironmentPreparer requires nothing but a Docker on PATH: the
// host kernel runs the containers directly, so there is no VM to manage and
// nothing to install on the user's behalf.
type linuxHostEnvironmentPreparer struct{}

func (linuxHostEnvironmentPreparer) Platform() HostPlatform { return HostPlatformLinux }

func (linuxHostEnvironmentPreparer) EnsureReady(_ context.Context, _ PrepareOptions) error {
	return requireHostContainerEngine()
}

func (linuxHostEnvironmentPreparer) EnsureStartable(
	_ context.Context, _, _ io.Writer,
) error {
	return nil
}

// ContainerHostRunning is always true on Linux: the containers run on this
// host's kernel, so there is nothing between them and the caller that can be
// stopped on its own.
func (linuxHostEnvironmentPreparer) ContainerHostRunning(_ context.Context) (bool, error) {
	return true, nil
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
	// Re-check prerequisites that can lapse between Prepare and Start.
	// Without this the failure surfaces later as an opaque engine error.
	if err := runtime.preparer.EnsureStartable(ctx, out, outErr); err != nil {
		return err
	}
	startConfig, err := runtime.podmanStartConfig(runtimeConfig.RuntimeConfig)
	if err != nil {
		return err
	}
	if err := prepareHostDataDir(startConfig.DataDir); err != nil {
		return err
	}
	if err := runtime.install().Start(ctx, out, outErr, startConfig); err != nil {
		return err
	}
	runtime.endpoint = &RuntimeEndpoint{DBPort: startConfig.ContainerDBPort}

	return nil
}

// prepareHostDataDir lets the unprivileged UDF sandbox enter the data
// directory under a rootful engine, where the launcher user is not the
// container's root. The parent work directory keeps other host users out.
//
// It also creates the directories users stage files in before Nano can create
// them as root, so staging needs no elevated rights.
func prepareHostDataDir(dataDir string) error {
	if err := os.MkdirAll(dataDir, dataDirMode); err != nil {
		return fmt.Errorf("failed to create Nano data directory %s: %w", dataDir, err)
	}
	if err := os.Chmod(dataDir, dataDirMode); err != nil {
		return fmt.Errorf("failed to set permissions on Nano data directory %s: %w", dataDir, err)
	}
	for _, stagingDir := range userStagingDirs {
		if err := createUserStagingDir(dataDir, stagingDir); err != nil {
			return err
		}
	}

	return nil
}

// createUserStagingDir creates the missing components of relativePath below
// dataDir. Components that already exist are left as they are: Nano may own
// them in a deployment it initialized before the launcher created them.
func createUserStagingDir(dataDir string, relativePath string) error {
	path := dataDir
	for component := range strings.SplitSeq(relativePath, string(filepath.Separator)) {
		path = filepath.Join(path, component)
		err := os.Mkdir(path, dataDirMode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if errors.Is(err, os.ErrPermission) {
			slog.Warn("cannot create a user staging directory inside a directory the "+
				"database owns; staging files there needs elevated rights", "path", path)

			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to create staging directory %s: %w", path, err)
		}
		if err := os.Chmod(path, dataDirMode); err != nil {
			return fmt.Errorf("failed to set permissions on staging directory %s: %w", path, err)
		}
	}

	return nil
}

func (runtime *HostRuntime) Stop(ctx context.Context, out, outErr io.Writer) error {
	return runtime.install().Stop(ctx, out, outErr)
}

// Status answers without starting anything, so observing a deployment causes no
// side effects on the host - `exasol status` above all, which is bounded by a
// short timeout a container host would not start within.
func (runtime *HostRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	status, err := runtime.install().Status(ctx, nil, nil)
	if err != nil {
		// A probe that could not reach the container host answers nothing about
		// the container, but a stopped host settles it: nothing runs inside one.
		hostRunning, hostErr := runtime.preparer.ContainerHostRunning(ctx)
		if hostErr == nil && !hostRunning {
			return &RuntimeStatus{Running: false}, nil
		}

		return nil, err
	}

	return &RuntimeStatus{Running: status.Running}, nil
}

// EnsureQueryable re-runs the platform's approval-free prerequisites so the
// runtime can be observed.
func (runtime *HostRuntime) EnsureQueryable(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	return runtime.preparer.EnsureStartable(ctx, out, outErr)
}

func (runtime *HostRuntime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	install := runtime.install()
	if err := install.Destroy(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to remove deployment: %w", err)
	}

	err := os.RemoveAll(runtime.paths.Root)
	if err != nil && errors.Is(err, os.ErrPermission) {
		// A rootful engine leaves database files owned by root.
		dataDir := filepath.Join(runtime.paths.WorkDir, nanoDataDirName)
		if clearErr := install.ClearDataDir(ctx, out, outErr, dataDir); clearErr != nil {
			err = errors.Join(err, clearErr)
		} else {
			err = os.RemoveAll(runtime.paths.Root)
		}
	}
	if err != nil {
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

func (runtime *HostRuntime) RemoveBootstrapPassword() error {
	return localinstall.RemoveHostBootstrapPassword(runtime.paths.BootstrapDir)
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
	address := net.JoinHostPort(hostLoopbackHost, strconv.Itoa(endpoint.DBPort))
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

func (runtime *HostRuntime) OpenHostShell(
	context.Context,
	[]string,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return fmt.Errorf(
		"%s host runtime does not support host shells: %w",
		runtime.Platform(),
		ErrHostShellUnsupported,
	)
}

func (runtime *HostRuntime) OpenContainerShell(
	context.Context,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return fmt.Errorf(
		"%s host runtime does not support container shells: %w",
		runtime.Platform(),
		ErrContainerShellUnsupported,
	)
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
	if isPeerReachedDialError(err) {
		return PortStateRefused
	}
	// The error slipped past every well-known classifier, so the network
	// path cannot be shown to work; logging the underlying error keeps
	// future occurrences diagnosable from deployment.log alone.
	slog.Debug("classifying host port dial error as blocked",
		"error", err.Error(), "errorType", fmt.Sprintf("%T", err))

	return PortStateBlocked
}

// Winsock error codes for the dial outcomes that prove a peer answered.
// syscall's POSIX-named errnos are unusable for this on Windows: there they
// are invented APPLICATION_ERROR placeholders that Winsock never returns,
// and Errno.Is bridges only the os.Err* sentinels, so matching them silently
// misses every real Windows dial error. The numeric codes are matched on
// every platform because no other platform issues errnos in this range,
// which keeps the Windows path exercisable by tests that do not run there.
const (
	wsaeConnAborted = 10053
	wsaeConnReset   = 10054
	wsaeNotConn     = 10057
	wsaeConnRefused = 10061
)

// isPeerReachedDialError reports whether a failed dial still proves the
// host-to-container network path carries traffic: the peer refused the
// connection or tore it down, rather than the packets going unanswered. A
// port with no listener yet, as during database startup, lands here.
func isPeerReachedDialError(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}

	return errno == wsaeConnRefused || errno == wsaeConnReset ||
		errno == wsaeConnAborted || errno == wsaeNotConn
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
		hostContainerEngine,
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
		// Publish on 127.0.0.1 so the database is not exposed beyond the host.
		ContainerDBBindHost: hostLoopbackHost,
		DataDir:             filepath.Join(runtime.paths.WorkDir, nanoDataDirName),
		InitParams:          append([]string(nil), nanoInitParams...),
		VersionCheck:        runtimeConfig.VersionCheck,
		SLCs:                runtimeConfig.SLCs,
		BootstrapDir:        runtime.paths.BootstrapDir,
		InitialPassword:     runtimeConfig.InitialPassword,
	}, nil
}

func resolveHostPodmanDBPort(ports string) (int, error) {
	hostDBPort := 0
	dbPortConfigured := false
	if strings.TrimSpace(ports) == "" {
		return 0, errors.New("local database port must be configured as a positive concrete port")
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
	if !dbPortConfigured {
		return 0, errors.New("local database port must be configured as a positive concrete port")
	}

	return hostDBPort, nil
}
