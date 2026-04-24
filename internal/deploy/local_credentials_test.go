// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestEnsureLocalSecrets_WritesLauncherDefaults(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword:      "unexpected-db-password",
		AdminUiPassword: "unexpected-ui-password",
	}); err != nil {
		t.Fatalf("expected legacy secrets fixture to be written, got %v", err)
	}

	secrets, err := ensureLocalSecrets(deployment)
	if err != nil {
		t.Fatalf("expected local secrets to be ensured, got %v", err)
	}
	if secrets.DbPassword != localDefaultDatabasePassword {
		t.Fatalf(
			"expected db password %q, got %q",
			localDefaultDatabasePassword,
			secrets.DbPassword,
		)
	}
	if secrets.AdminUiPassword != localDefaultAdminUIPassword {
		t.Fatalf(
			"expected admin UI password %q, got %q",
			localDefaultAdminUIPassword,
			secrets.AdminUiPassword,
		)
	}
}

func TestEnsureLocalDatabaseCredentials_UsesCentralizedDefaults(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	if err := ensureLocalDatabaseCredentials(context.Background(), deployment); err != nil {
		t.Fatalf("expected local database credentials to be ensured, got %v", err)
	}

	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		t.Fatalf("expected local secrets to be readable, got %v", err)
	}
	if secrets.DbPassword != localDefaultDatabasePassword {
		t.Fatalf(
			"expected db password %q, got %q",
			localDefaultDatabasePassword,
			secrets.DbPassword,
		)
	}
}
