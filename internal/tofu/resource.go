// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// ResolveBinaryPath resolves the runtime tofu binary path.
func ResolveBinaryPath(ctx context.Context) (string, error) {
	spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
	if err != nil {
		return "", err
	}

	manager, err := runtimeartifacts.NewResourceManager(spec)
	if err != nil {
		return "", err
	}

	return manager.Request(ctx, "tofu")
}
