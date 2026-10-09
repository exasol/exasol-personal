// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"

	"github.com/exasol/exasol-personal/internal/localinstall"
)

type sidecarHost struct {
	name         string
	architecture string
	provider     localinstall.SidecarProvider
	running      func(context.Context) (bool, error)
}

// Deployments record sidecar enablement and serve it from their own hosts
// once those hosts can run containers.
func (*localBackend) SidecarHosts(context.Context) ([]sidecarHost, error) {
	return nil, nil
}

func (*tofuBackend) SidecarHosts(context.Context) ([]sidecarHost, error) {
	return nil, nil
}
