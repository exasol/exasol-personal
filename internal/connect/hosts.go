// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
)

func databaseAddress(info *config.ConnectionInfo) (string, int, error) {
	if !info.AllowAllNodes {
		return info.Host, info.DBPort, nil
	}

	for _, endpoint := range info.Endpoints {
		if !endpoint.Eligible {
			continue
		}
		hosts, err := config.ConnectionHosts(info.Endpoints, endpoint.DBPort)
		if err != nil {
			return "", 0, err
		}

		return strings.Join(hosts, ","), endpoint.DBPort, nil
	}

	return "", 0, errors.New("no eligible public database endpoints: " +
		"check the infrastructure preset public addresses and database ports")
}
