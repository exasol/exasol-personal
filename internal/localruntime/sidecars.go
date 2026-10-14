// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

func (runtime *HostRuntime) Sidecars(_ context.Context) (*localinstall.SidecarRuntime, error) {
	owner, err := localinstall.ContainerName(runtime.deployment)
	if err != nil {
		return nil, err
	}
	manager, err := localinstall.NewSidecarRuntime(
		runtime.preparer.NewExecutionEnvironment(runtime.runCmd()), owner,
		localinstall.IdentityRuntimePath(SidecarEnvDir(runtime.deployment)),
	)
	if err != nil {
		return nil, err
	}
	manager.Salt, err = config.SidecarSalt(runtime.deployment)

	return manager, err
}

func (runtime *HostRuntime) SidecarHostRunning(ctx context.Context) (bool, error) {
	return runtime.preparer.ContainerHostRunning(ctx)
}

// OpenPorts has nothing to open: containers run directly on the user's
// machine, where the container runtime already binds the requested address.
func (*HostRuntime) OpenPorts(context.Context, []sidecar.PublishedPort) error {
	return nil
}

func (*HostRuntime) OpenedPorts(context.Context) ([]sidecar.PublishedPort, error) {
	return nil, nil
}
