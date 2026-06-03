// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// ResolveBinaryPath resolves the runtime tofu binary path for the given deployment directory.
func ResolveBinaryPath(ctx context.Context, deploymentRoot string) (string, error) {
	spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
	if err != nil {
		return "", err
	}

	manager := runtimeartifacts.NewResourceManager(spec, deploymentRoot)

	return manager.Request(ctx, "tofu")
}
