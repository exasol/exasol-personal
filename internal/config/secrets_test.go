// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecretsFilePath_MissingFileReturnsError(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())

	_, err := SecretsFilePath(deployment)

	require.ErrorContains(t, err, "secrets file not found")
}

func TestReadSecrets_MissingFileReturnsError(t *testing.T) {
	t.Parallel()

	deployment := NewDeploymentDir(t.TempDir())

	_, err := ReadSecrets(deployment)

	require.ErrorContains(t, err, "secrets file not found")
}

func TestWriteThenReadSecrets_RoundTrips(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)

	require.NoError(t, WriteSecrets(root, &Secrets{
		DbPassword:      "db-secret",
		AdminUiPassword: "admin-secret",
	}))

	secrets, err := ReadSecrets(deployment)

	require.NoError(t, err)
	require.Equal(t, "db-secret", secrets.DbPassword)
	require.Equal(t, "admin-secret", secrets.AdminUiPassword)
}

func TestWriteSecrets_NilDefaultsToEmptySecrets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	deployment := NewDeploymentDir(root)

	require.NoError(t, WriteSecrets(root, nil))

	secrets, err := ReadSecrets(deployment)

	require.NoError(t, err)
	require.Empty(t, secrets.DbPassword)
}
