// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

func initLocalDeploymentForInfoTest(t *testing.T) config.DeploymentDir {
	t.Helper()
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, InitDeployment(
		testManagerContext(t),
		deployment,
		InitOptions{
			InfrastructurePreset: PresetRef{Name: "local"},
			InstallationPreset:   PresetRef{Name: "local"},
			InfraVars:            map[string]string{},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
	))

	return deployment
}

func writeSidecarsForInfoTest(t *testing.T, deployment config.DeploymentDir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(deployment.SidecarsPath(), []byte(content), 0o600))
}

func TestInfoReportsSavedSidecars(t *testing.T) {
	t.Parallel()
	// Given
	deployment := initLocalDeploymentForInfoTest(t)
	writeSidecarsForInfoTest(t, deployment, `version: 1
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
  plain:
    container:
      image: example.org/plain:1
      ports:
        - containerPort: 8080
          hostPort: 18080
  internal:
    enabled: false
    container:
      image: example.org/internal:1
      ports:
        - containerPort: 9090
`)
	// When
	report, err := GetDeploymentInfoReport(testManagerContext(t), deployment)
	// Then
	require.NoError(t, err)
	require.Equal(t, []SidecarSummary{
		{Name: "internal", Enabled: false, Endpoints: []SidecarEndpoint{}},
		{Name: "mcp", Enabled: true, Endpoints: []SidecarEndpoint{
			{IP: "127.0.0.1", Port: 14896, URL: "http://127.0.0.1:14896/mcp"},
		}},
		{Name: "plain", Enabled: true, Endpoints: []SidecarEndpoint{
			{IP: "127.0.0.1", Port: 18080},
		}},
	}, report.Sidecars)
}

func TestInfoReportsNoSavedSidecarsAsEmpty(t *testing.T) {
	t.Parallel()
	// Given
	deployment := initLocalDeploymentForInfoTest(t)
	// When
	report, err := GetDeploymentInfoReport(testManagerContext(t), deployment)
	// Then
	require.NoError(t, err)
	require.NotNil(t, report.Sidecars)
	require.Empty(t, report.Sidecars)
}

func TestInfoOmitsSidecarsForDeploymentsWithoutSidecarSupport(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	initDeploymentForInfoTest(t, deployment)
	// When
	report, err := GetDeploymentInfoReport(testManagerContext(t), deployment)
	// Then
	require.NoError(t, err)
	require.Nil(t, report.Sidecars)
}
