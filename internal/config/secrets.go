// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"log/slog"
	"path/filepath"
)

//nolint:gosec // gosec thinks this is a password
const secretsFileName = "secrets.json"

type Secrets struct {
	DbPassword           string `json:"dbPassword"`
	AdminUiPassword      string `json:"adminUiPassword,omitempty"`
	AiLabScsPassword     string `json:"aiLabScsPassword,omitempty"`
	AiLabJupyterPassword string `json:"aiLabJupyterPassword,omitempty"`
}

func SecretsFilePath(deployment DeploymentDir) (string, error) {
	secretsPath, exists, err := findExistingFile(deployment.Root(), secretsFileName)
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

	return secretsPath, nil
}

func ReadSecrets(deployment DeploymentDir) (*Secrets, error) {
	secretsPath, err := SecretsFilePath(deployment)
	if err != nil {
		return nil, err
	}

	slog.Debug("reading secrets file", "file", secretsPath)

	return readConfig[Secrets](secretsPath, "secrets")
}

func WriteSecrets(deploymentDir string, secrets *Secrets) error {
	if secrets == nil {
		secrets = &Secrets{}
	}

	return writeConfig(secrets, filepath.Join(deploymentDir, secretsFileName), "secrets")
}
