// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"log/slog"
)

//nolint:gosec // gosec thinks this is a password
const secretsGlob = "secrets-exasol-*.json"

type Secrets struct {
	DbPassword      string `json:"dbPassword"`
	AdminUiPassword string `json:"adminUiPassword"`
}

func GetSecretsFilePath(deploymentDir string) (string, error) {
	filepath, err := findGlob(deploymentDir, secretsGlob)
	if err != nil {
		return "", fmt.Errorf("failed to get the secrets file path: %w", err)
	}

	return filepath, nil
}

func ReadSecrets(deploymentDir string) (*Secrets, error) {
	filepath, err := GetSecretsFilePath(deploymentDir)
	if err != nil {
		return nil, err
	}

	slog.Debug("reading secrets file", "file", filepath)

	return readConfig[Secrets](filepath, "secrets")
}
