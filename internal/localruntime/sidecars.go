// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"

	"github.com/exasol/exasol-personal/internal/localinstall"
)

func (runtime *HostRuntime) Sidecars(_ context.Context) (localinstall.SidecarManager, error) {
	owner, err := localinstall.ContainerName(runtime.deployment)
	if err != nil {
		return nil, err
	}

	return localinstall.NewSidecarRuntime(
		runtime.preparer.NewExecutionEnvironment(runtime.runCmd()), owner,
	)
}

func (runtime *HostRuntime) SidecarHostRunning(ctx context.Context) (bool, error) {
	return runtime.preparer.ContainerHostRunning(ctx)
}
