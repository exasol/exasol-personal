// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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
	deployment config.DeploymentDir
	paths      runtimePaths
	manager    *runtimeartifacts.Manager
	endpoint   *RuntimeEndpoint
}

// NewHostLinuxRuntime creates a Linux host runtime. The manager may be nil for operations
// that never invoke the runner, such as destroying an unprepared deployment.
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

// Prepare validates Linux host prerequisites and creates the runtime work directory.
// VM sizing is provided when the runtime starts.
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
	install := localinstall.NewPodmanInstall(
		runtime.Deployment(), runtime.manager, runtime.runCmd(),
		SLCStagingDir(runtime.Deployment()), SLCStatusPath(runtime.Deployment()),
	)

	if err := install.Start(ctx, out, outErr, startConfig); err != nil {
		return err
	}
	runtime.endpoint = &RuntimeEndpoint{DBPort: startConfig.ContainerDBPort}

	return nil
}

func (runtime *LinuxHostRuntime) Stop(ctx context.Context, out, outErr io.Writer) error {
	install := localinstall.NewPodmanInstall(
		runtime.Deployment(), runtime.manager, runtime.runCmd(),
		SLCStagingDir(runtime.Deployment()), SLCStatusPath(runtime.Deployment()),
	)

	return install.Stop(ctx, out, outErr)
}

func (*LinuxHostRuntime) Status(_ context.Context) (*RuntimeStatus, error) {
	// Host status reporting is outside the current minimal lifecycle.
	return &RuntimeStatus{Running: true}, nil
}

func (runtime *LinuxHostRuntime) Destroy(ctx context.Context, out, outErr io.Writer) error {
	install := localinstall.NewPodmanInstall(
		runtime.Deployment(), runtime.manager, runtime.runCmd(),
		SLCStagingDir(runtime.Deployment()), SLCStatusPath(runtime.Deployment()),
	)

	if err := install.Destroy(ctx, out, outErr); err != nil {
		return fmt.Errorf("failed to remove deployment: %w", err)
	}

	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

func (runtime *LinuxHostRuntime) ReadEndpoints() (*VMRuntimeEndpoint, error) {
	if runtime.endpoint == nil {
		return nil, errors.New("linux host runtime endpoint is unavailable before start")
	}

	return &VMRuntimeEndpoint{RuntimeEndpoint: *runtime.endpoint}, nil
}

func (*LinuxHostRuntime) HealthCheck(context.Context) (*HealthCheckResult, error) {
	return nil, errors.New("not implemented")
}

func (*LinuxHostRuntime) runCmd() []string {
	return []string{}
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
