// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

const mcpTestDefinition = `version: 1
sidecars:
  mcp:
    container:
      image: example.org/mcp:1
      ports:
        - name: mcp
          containerPort: 4896
          hostPort: 14896
    endpoints:
      - port: mcp
        scheme: http
        path: /mcp
`

func TestMCPServerURLUsesTheSavedDefinition(t *testing.T) {
	t.Parallel()
	// Given
	deployment := initLocalDeploymentForInfoTest(t)
	writeSidecarsForInfoTest(t, deployment, mcpTestDefinition)
	// When
	endpoint, err := MCPServerURL(t.Context(), deployment)
	// Then
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:14896/mcp", endpoint)
}

func TestMCPServerURLRequiresAnEnabledServer(t *testing.T) {
	t.Parallel()
	for name, definition := range map[string]string{
		"missing": "version: 1\nsidecars: {}\n",
		"disabled": strings.Replace(
			mcpTestDefinition, "  mcp:\n", "  mcp:\n    enabled: false\n", 1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given
			deployment := initLocalDeploymentForInfoTest(t)
			writeSidecarsForInfoTest(t, deployment, definition)
			// When
			_, err := MCPServerURL(t.Context(), deployment)
			// Then
			require.ErrorIs(t, err, ErrMCPServerNotEnabled)
		})
	}
}

func TestMCPServerURLRequiresAPublishedEndpoint(t *testing.T) {
	t.Parallel()
	// Given
	deployment := initLocalDeploymentForInfoTest(t)
	writeSidecarsForInfoTest(t, deployment,
		"version: 1\nsidecars:\n  mcp:\n    container:\n      image: example.org/mcp:1\n")
	// When
	_, err := MCPServerURL(t.Context(), deployment)
	// Then
	require.ErrorIs(t, err, ErrMCPServerNoEndpoint)
}

func TestMCPServerURLRejectsDeploymentsWithoutSidecarSupport(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)
	// When
	_, err := MCPServerURL(t.Context(), deployment)
	// Then
	require.ErrorIs(t, err, ErrSidecarsUnsupported)
}

func TestSuggestMCPServerUntilItIsEnabled(t *testing.T) {
	t.Parallel()
	// Given
	deployment := initLocalDeploymentForInfoTest(t)
	cloud := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, cloud)
	// When / Then
	require.Equal(t, CatalogSupports(MCPSidecarName), SuggestMCPServer(deployment))
	require.False(t, SuggestMCPServer(cloud))
	writeSidecarsForInfoTest(t, deployment, mcpTestDefinition)
	require.False(t, SuggestMCPServer(deployment))
	writeSidecarsForInfoTest(t, deployment, strings.Replace(
		mcpTestDefinition, "  mcp:\n", "  mcp:\n    enabled: false\n", 1,
	))
	require.Equal(t, CatalogSupports(MCPSidecarName), SuggestMCPServer(deployment))
}
