// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	localDefaultAdminUIPassword = ""
)

var ensureLocalDatabaseCredentialsFn = ensureLocalDatabaseCredentials

func ensureLocalSecrets(deployment config.DeploymentDir) (*config.Secrets, error) {
	secretsPath := deployment.SecretsPath()
	secrets := &config.Secrets{}

	if _, err := os.Stat(secretsPath); err == nil {
		existing, readErr := config.ReadSecrets(deployment)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read local deployment secrets: %w", readErr)
		}
		secrets = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to inspect local deployment secrets: %w", err)
	}

	normalized := &config.Secrets{
		DbPassword:      localDefaultDatabasePassword,
		AdminUiPassword: localDefaultAdminUIPassword,
	}
	if normalized.DbPassword != strings.TrimSpace(secrets.DbPassword) ||
		normalized.AdminUiPassword != strings.TrimSpace(secrets.AdminUiPassword) {
		if err := config.WriteSecrets(deployment.Root(), normalized); err != nil {
			return nil, fmt.Errorf("failed to write local deployment secrets: %w", err)
		}
	}

	return normalized, nil
}

func ensureLocalDatabaseCredentials(_ context.Context, deployment config.DeploymentDir) error {
	_, err := ensureLocalSecrets(deployment)

	return err
}
