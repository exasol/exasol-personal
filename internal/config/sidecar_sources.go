// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"cmp"
	"errors"
	"os"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

func SidecarSources(deployment DeploymentDir) (sidecar.Sources, error) {
	database := map[string]string{"host": "database", "port": "8563"}
	if _, err := os.Stat(deployment.NodeDetailsPath()); err == nil {
		info, err := ReadDeploymentInfo(deployment)
		if err != nil {
			return nil, err
		}
		if info.Connection != nil {
			database["username"] = cmp.Or(info.Connection.Username, "sys")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, err := os.Stat(deployment.SecretsPath()); err == nil {
		secrets, err := ReadSecrets(deployment)
		if err != nil {
			return nil, err
		}
		database["password"] = secrets.DbPassword
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return sidecar.Sources{sidecar.DatabaseSource: database}, nil
}
