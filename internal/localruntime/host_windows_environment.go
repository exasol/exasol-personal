// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// windowsHostEnvironmentPreparer satisfies the Windows prerequisites for the
// shared HostRuntime: a Docker on PATH whose daemon hosts the containers.
// The launcher neither installs Docker nor manages its daemon.
type windowsHostEnvironmentPreparer struct{}

// NewHostWindowsRuntime creates a Windows host runtime. The manager may be
// nil for operations that never invoke the container engine, such as
// destroying a deployment that was never prepared.
func NewHostWindowsRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *HostRuntime {
	return newHostRuntime(deployment, manager, windowsHostEnvironmentPreparer{})
}

func (windowsHostEnvironmentPreparer) Platform() HostPlatform { return HostPlatformWindows }

func (windowsHostEnvironmentPreparer) EnsureReady(_ context.Context, _ PrepareOptions) error {
	return requireHostContainerEngine()
}

func (windowsHostEnvironmentPreparer) EnsureStartable(
	_ context.Context, _, _ io.Writer,
) error {
	return requireHostContainerEngine()
}

// ContainerHostRunning reports whether the Docker daemon answers. A stopped
// daemon, such as Docker Desktop not running, hosts no containers.
func (windowsHostEnvironmentPreparer) ContainerHostRunning(ctx context.Context) (bool, error) {
	if err := requireHostContainerEngine(); err != nil {
		return false, err
	}
	//nolint:gosec // fixed engine name
	output, err := exec.CommandContext(ctx, string(hostContainerEngine), "info").CombinedOutput()
	if err != nil {
		slog.Warn("container engine is not answering; treating it as stopped",
			"engine", hostContainerEngine, "error", err,
			"output", strings.TrimSpace(string(output)))

		return false, nil
	}

	return true, nil
}

func (windowsHostEnvironmentPreparer) NewExecutionEnvironment(
	runtimeExec []string,
) localinstall.ExecutionEnvironment {
	return &windowsHostExecutionEnvironment{
		DirectExecutionEnvironment: localinstall.NewDirectExecutionEnvironment(runtimeExec),
	}
}

// windowsHostExecutionEnvironment skips Sync: Windows offers no host-side
// `sync`, and Docker Desktop's VM is not reachable to flush from inside.
type windowsHostExecutionEnvironment struct {
	*localinstall.DirectExecutionEnvironment
}

func (*windowsHostExecutionEnvironment) Sync(context.Context, io.Writer, io.Writer) error {
	return nil
}
