// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
)

func databaseHosts(info *config.ConnectionInfo) (string, error) {
	if !info.MultiNode {
		return info.Host, nil
	}

	hosts, err := config.ConnectionHosts(info.Endpoints, info.DBPort)
	if err != nil {
		return "", err
	}
	if len(hosts) == 0 {
		return "", errors.New("multi-node deployment has no connectable public database endpoints")
	}

	return strings.Join(hosts, ","), nil
}
