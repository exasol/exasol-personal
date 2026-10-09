// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"runtime"

	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

type sidecarHost struct {
	name         string
	architecture string
	provider     localinstall.SidecarProvider
	running      func(context.Context) (bool, error)
}

func localSidecarHost(selected localruntime.Runtime) sidecarHost {
	return sidecarHost{
		name: "local", architecture: runtime.GOARCH, provider: selected,
		running: func(ctx context.Context) (bool, error) {
			status, err := selected.Status(ctx)
			if err != nil {
				return false, err
			}

			return status.Running, nil
		},
	}
}

func (backend *localBackend) SidecarHosts(context.Context) ([]sidecarHost, error) {
	host := localSidecarHost(backend.runtime)
	host.architecture = backend.goarch

	return []sidecarHost{host}, nil
}

// Cloud deployments record sidecar enablement and serve it from their own
// hosts once those hosts can run containers.
func (*tofuBackend) SidecarHosts(context.Context) ([]sidecarHost, error) {
	return nil, nil
}
