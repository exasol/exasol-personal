// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

func savedSidecarHostPorts(document sidecar.Document, except string) map[int]string {
	saved := map[int]string{}
	for _, name := range document.Names() {
		if name == except {
			continue
		}
		for _, port := range document.Sidecars[name].Container.Ports {
			if port.HostPort > 0 {
				saved[port.HostPort] = fmt.Sprintf("sidecar %q", name)
			}
		}
	}

	return saved
}
