// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"log/slog"
)

//nolint:gosec // gosec thinks this is a password
const secretsFileName = "secrets.json"

type Secrets struct {
	DbPassword      string `json:"dbPassword"`
	AdminUiPassword string `json:"adminUiPassword"`
}

func SecretsFilePath(deployment DeploymentDir) (string, error) {
	filepath, exists, err := findExistingFile(deployment.Root(), secretsFileName)
	if err != nil {
		return "", fmt.Errorf("failed to get the secrets file path: %w", err)
	}
	if !exists {
		return "", fmt.Errorf(
			"secrets file not found in deployment directory: expected %q in %s",
			secretsFileName,
			deployment.Root(),
		)
	}

	return filepath, nil
}

func ReadSecrets(deployment DeploymentDir) (*Secrets, error) {
	filepath, err := SecretsFilePath(deployment)
	if err != nil {
		return nil, err
	}

	slog.Debug("reading secrets file", "file", filepath)

	return readConfig[Secrets](filepath, "secrets")
}
