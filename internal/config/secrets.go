// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
)

const (
	//nolint:gosec // gosec thinks this is a password
	secretsFileName = "secrets.json"
	secretsFileMode = 0o600
)

type Secrets struct {
	DbPassword      string `json:"dbPassword"`
	AdminUiPassword string `json:"adminUiPassword,omitempty"`
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

	data, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("failed to encode secrets: %w", err)
	}
	path := filepath.Join(deploymentDir, secretsFileName)
	if err := WriteFileAtomically(path, data, secretsFileMode); err != nil {
		return fmt.Errorf("failed to write secrets file %s: %w", path, err)
	}
	slog.Debug("new config file written", "type", "secrets", "path", path)

	return nil
}
