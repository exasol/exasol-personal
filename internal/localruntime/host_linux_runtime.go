// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// LinuxHostRuntime owns Linux-specific prerequisites and delegates the database
// container lifecycle to localinstall.
type LinuxHostRuntime struct {
	deployment  config.DeploymentDir
	paths       runtimePaths
	manager     *runtimeartifacts.Manager
	endpoint    *RuntimeEndpoint
	runtimeExec []string
	dialContext func(context.Context, string, string) (net.Conn, error)
}

// manager may be nil for operations that never need to invoke the runner
// (e.g. Destroy on a deployment that was never prepared).
func NewHostLinuxRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *LinuxHostRuntime {
	return &LinuxHostRuntime{
		deployment: deployment,
		paths:      newRuntimePaths(deployment),
		manager:    manager,
	}
}

func (runtime *LinuxHostRuntime) Deployment() config.DeploymentDir {
	return runtime.deployment
}

// VM sizing (CPU/memory/data disk) is not a Prepare concern: it's passed
// directly as RunCommand args for "start".
func (runtime *LinuxHostRuntime) Prepare(_ context.Context, _, _ io.Writer) error {
	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf("'podman' is required to run exasol locally on this platform: %w", err)
	}
	if err := os.MkdirAll(runtime.paths.WorkDir, dirMode); err != nil {
		return fmt.Errorf("failed to create local runtime directory: %w", err)
	}

	return nil
}

func (runtime *LinuxHostRuntime) Start(
	ctx context.Context,
	out, outErr io.Writer,
	runtimeConfig VMConfig,
) error {
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

func (runtime *LinuxHostRuntime) Stop(ctx context.Context, out, outErr io.Writer) error {
	return runtime.install().Stop(ctx, out, outErr)
}

func (runtime *LinuxHostRuntime) Status(ctx context.Context) (*RuntimeStatus, error) {
	status, err := runtime.install().Status(ctx, nil, nil)
	if err != nil {
		return nil, err
	}

	return &RuntimeStatus{Running: status.Running}, nil
}

func (runtime *LinuxHostRuntime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	if err := runtime.install().Destroy(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to remove deployment: %w", err)
	}

	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

func (runtime *LinuxHostRuntime) WorkaroundNanoStartupDurability(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	environment := localinstall.NewDirectExecutionEnvironment(runtime.runCmd())
	if err := environment.Sync(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to apply Nano startup durability workaround: %w", err)
	}

	return nil
}

func (runtime *LinuxHostRuntime) ReadEndpoints() (*VMRuntimeEndpoint, error) {
	if runtime.endpoint == nil {
		info, err := config.ReadDeploymentInfo(runtime.Deployment())
		if err != nil {
			return nil, fmt.Errorf("linux host runtime endpoint is unavailable: %w", err)
		}
		if info.Connection == nil || info.Connection.DBPort <= 0 || info.Connection.DBPort > 65535 {
			return nil, errors.New(
				"linux host runtime deployment information has no valid database endpoint",
			)
		}
		runtime.endpoint = &RuntimeEndpoint{DBPort: info.Connection.DBPort}
	}

	return &VMRuntimeEndpoint{RuntimeEndpoint: *runtime.endpoint}, nil
}

func (runtime *LinuxHostRuntime) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
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

func (*LinuxHostRuntime) OpenHostShell(
	context.Context,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return ErrHostShellUnsupported
}

func (*LinuxHostRuntime) OpenContainerShell(
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

	return PortStateBlocked
}

func (runtime *LinuxHostRuntime) install() *localinstall.PodmanInstall {
	return localinstall.NewPodmanInstall(
		runtime.Deployment(), runtime.manager, runtime.runCmd(),
		SLCStagingDir(runtime.Deployment()), SLCStatusPath(runtime.Deployment()),
	)
}

func (runtime *LinuxHostRuntime) runCmd() []string {
	return runtime.runtimeExec
}

func (runtime *LinuxHostRuntime) podmanStartConfig(
	runtimeConfig RuntimeConfig,
) (localinstall.StartConfig, error) {
	hostDBPort, err := resolveLinuxHostDBPort(runtimeConfig.Ports)
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

func resolveLinuxHostDBPort(ports string) (int, error) {
	hostDBPort := nanoDBPort
	dbPortConfigured := false
	if strings.TrimSpace(ports) == "" {
		return hostDBPort, nil
	}

	for _, rawEntry := range strings.Split(ports, ",") {
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
