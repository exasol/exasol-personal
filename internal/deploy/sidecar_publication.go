// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

func (backend *localBackend) OpenPorts(
	ctx context.Context,
	ports []sidecar.PublishedPort,
) error {
	return backend.runtime.OpenPorts(ctx, ports)
}

func (backend *localBackend) OpenedPorts(
	ctx context.Context,
) ([]sidecar.PublishedPort, error) {
	return backend.runtime.OpenedPorts(ctx)
}

// A cloud backend opens endpoints once it serves hosts of its own.

func (*tofuBackend) OpenPorts(context.Context, []sidecar.PublishedPort) error {
	return nil
}

func (*tofuBackend) OpenedPorts(context.Context) ([]sidecar.PublishedPort, error) {
	return nil, nil
}
