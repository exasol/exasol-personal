// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

// A backend opens endpoints once it serves hosts of its own.
func (*localBackend) OpenPorts(context.Context, []sidecar.PublishedPort) error {
	return nil
}

func (*localBackend) OpenedPorts(context.Context) ([]sidecar.PublishedPort, error) {
	return nil, nil
}

func (*tofuBackend) OpenPorts(context.Context, []sidecar.PublishedPort) error {
	return nil
}

func (*tofuBackend) OpenedPorts(context.Context) ([]sidecar.PublishedPort, error) {
	return nil, nil
}
