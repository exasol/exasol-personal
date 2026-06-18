// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadToken_FromEnv(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv(TokenEnv, "exa_pat_env")

	// Env token is returned without reading deployment secrets.
	token, err := LoadToken(config.DeploymentDir{})
	require.NoError(t, err)
	require.Equal(t, "exa_pat_env", token)
}

func TestRequireToken_HintMentionsEnvVar(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv(TokenEnv, "")

	dir := t.TempDir()
	require.NoError(t, config.WriteSecrets(dir, &config.Secrets{DbPassword: "x"}))

	_, err := RequireToken(config.NewDeploymentDir(dir))
	require.Error(t, err)
	require.Contains(t, err.Error(), TokenEnv)
	require.NotContains(t, err.Error(), "exasol saas token")
}

func TestMaskToken(t *testing.T) {
	t.Parallel()
	require.Equal(t, "(none)", MaskToken(""))
	require.Equal(t, "********wxyz", MaskToken("abcdefghwxyz"))
}
