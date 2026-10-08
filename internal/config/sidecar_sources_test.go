// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config_test

import (
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func TestSidecarSourcesUseInternalAddressAndDeploymentCredentials(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	info := []byte(`{"connection":{"host":"127.0.0.1","dbPort":12345,"username":"sys"}}`)
	require.NoError(t, os.WriteFile(deployment.NodeDetailsPath(), info, 0o600))
	require.NoError(t, config.WriteSecrets(
		deployment.Root(),
		&config.Secrets{DbPassword: "test-secret"},
	))
	// When
	sources, err := config.SidecarSources(deployment)
	// Then
	require.NoError(t, err)
	database := sources[sidecar.DatabaseSource]
	if database["host"] != "database" || database["port"] != "8563" ||
		database["username"] != "sys" || database["password"] != "test-secret" {
		t.Fatal("incorrect connection sources")
	}
}

func TestSidecarSourcesAllowMissingOptionalCredentials(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	// When
	sources, err := config.SidecarSources(deployment)
	// Then
	require.NoError(t, err)
	if _, exists := sources[sidecar.DatabaseSource]["password"]; exists {
		t.Fatal("missing password must remain unavailable")
	}
}
