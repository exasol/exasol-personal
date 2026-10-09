// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

func TestSidecarCommandFlagsAndArguments(t *testing.T) {
	t.Parallel()
	// Given
	command := newSidecarCommand(&CommonFlags{})
	// When / Then
	for _, name := range []string{"list", "enable", "status", "disable"} {
		child, _, err := command.Find([]string{name})
		if err != nil || child.Name() != name {
			t.Fatalf("missing %s", name)
		}
		for _, flag := range []string{"deployment", "deployment-dir", "json"} {
			if child.Flags().Lookup(flag) == nil {
				t.Fatalf("%s missing --%s", name, flag)
			}
		}
		args := []string{"example"}
		if name == "list" {
			args = nil
		}
		require.NoError(t, child.Args(child, args))
		require.Equal(t, name != "list", deploymentFileLoggingIsRequired(child))
		if child.Args(child, []string{"example", "extra"}) == nil {
			t.Fatal("extra argument accepted")
		}
	}
}

//nolint:paralleltest // Terminal queues are process-global.
func TestSidecarEnableCommandUsesSavedDefinition(t *testing.T) {
	// Given
	catalog := resources.SidecarCatalogYAML
	resources.SidecarCatalogYAML = []byte("invalid catalog")
	t.Cleanup(func() { resources.SidecarCatalogYAML = catalog })
	resetTerminalMessages()
	t.Cleanup(resetTerminalMessages)
	directory := writeInitializedDeployment(t, "backend: local\nlocal: {}\n")
	deployment := config.NewDeploymentDir(directory)
	if err := config.WriteSidecars(deployment, sidecar.Document{
		Version: 1,
		Containers: []sidecar.Container{{
			Name: "saved", Image: "caddy:2",
			Env: []sidecar.EnvVar{{Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{
				SecretKeyRef: &sidecar.SecretKeyRef{Name: sidecar.DatabaseSource, Key: "password"},
			}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	command := newSidecarCommand(&CommonFlags{})
	command.SetArgs([]string{"enable", "saved", "--deployment-dir", directory, "--json"})
	// When
	err := command.ExecuteContext(testManagerContext(t))
	document, readErr := config.ReadSidecars(deployment)
	// Then
	if err != nil || readErr != nil || len(document.Containers[0].Env) != 1 {
		t.Fatalf("saved definition: %+v, %v, %v", document, err, readErr)
	}
	var decoded deploy.SidecarResult
	require.NoError(t, json.Unmarshal([]byte(strings.Join(terminalOutputs, "")), &decoded))
	if !decoded.Enabled || len(decoded.Hosts) != 0 || decoded.Name != "saved" {
		t.Fatalf("result: %+v", decoded)
	}
}

func TestSidecarTextIncludesEndpointsAndFailure(t *testing.T) {
	t.Parallel()
	// Given
	result := deploy.SidecarResult{
		Name: "example", Enabled: true, Hosts: []deploy.SidecarHostResult{{
			Name:  "local",
			State: "exited", ExitCode: 1, Error: "startup failed",
			Reconciliation:     "pending",
			LastOperationError: "old cleanup failure",
			Endpoints:          []deploy.SidecarEndpoint{{IP: "::1", Port: 18080}},
		}},
	}
	// When
	text := formatSidecarResult(result)
	// Then
	for _, want := range []string{
		"example: enabled", "Host local: exited", "[::1]:18080", "Exit code: 1", "startup failed",
		"Reconciliation: pending", "Unresolved operation failure: old cleanup failure",
	} {
		require.Contains(t, text, want)
	}
}

//nolint:paralleltest // The embedded catalog is process-global.
func TestSidecarSavedOperationsIgnoreCatalog(t *testing.T) {
	// Given
	catalog := resources.SidecarCatalogYAML
	resources.SidecarCatalogYAML = []byte("invalid catalog")
	t.Cleanup(func() { resources.SidecarCatalogYAML = catalog })
	directory := writeInitializedDeployment(t, "backend: local\nlocal: {}\n")
	deployment := config.NewDeploymentDir(directory)
	require.NoError(t, config.WriteSidecars(deployment, sidecar.Document{
		Version: 1, Containers: []sidecar.Container{{Name: "custom", Image: "caddy:2"}},
	}))
	// When
	status, statusErr := deploy.GetSidecarStatus(testManagerContext(t), deployment, "custom")
	disabled, disableErr := deploy.DisableSidecar(testManagerContext(t), deployment, "custom")
	// Then
	require.NoError(t, statusErr)
	require.True(t, status.Enabled)
	require.NoError(t, disableErr)
	require.False(t, disabled.Enabled)
}

const architectureCatalogYAML = `version: 1
sidecars:
  here:
    description: Entry for this architecture
    architectures: [%s]
    container:
      image: docker.io/library/caddy:2-alpine
  elsewhere:
    description: Entry for another architecture
    architectures: [%s]
    container:
      image: docker.io/library/caddy:2-alpine
`

func withCatalog(t *testing.T, catalog string) {
	t.Helper()
	previous := resources.SidecarCatalogYAML
	resources.SidecarCatalogYAML = []byte(catalog)
	t.Cleanup(func() { resources.SidecarCatalogYAML = previous })
}

// architectureCatalog offers one entry this machine supports and one it does not.
func architectureCatalog(t *testing.T) string {
	t.Helper()
	other := "arm64"
	if runtime.GOARCH == "arm64" {
		other = "amd64"
	}

	return fmt.Sprintf(architectureCatalogYAML, runtime.GOARCH, other)
}

//nolint:paralleltest // The embedded catalog and terminal queues are process-global.
func TestSidecarListReportsAvailability(t *testing.T) {
	// Given
	withCatalog(t, architectureCatalog(t))
	t.Cleanup(resetTerminalMessages)
	resetTerminalMessages()
	directory := writeInitializedDeployment(t, "backend: local\nlocal: {}\n")
	selection := []string{"--deployment-dir", directory}
	enable := newSidecarCommand(&CommonFlags{})
	enable.SetArgs(append([]string{"enable", "here"}, selection...))
	require.NoError(t, enable.ExecuteContext(testManagerContext(t)))
	resetTerminalMessages()
	// When
	command := newSidecarCommand(&CommonFlags{})
	command.SetArgs(append([]string{"list", "--json"}, selection...))
	err := command.ExecuteContext(testManagerContext(t))
	// Then
	require.NoError(t, err)
	var listing []deploy.SidecarAvailability
	require.NoError(t, json.Unmarshal([]byte(strings.Join(terminalOutputs, "")), &listing))
	byName := map[string]deploy.SidecarAvailability{}
	for _, entry := range listing {
		byName[entry.Name] = entry
	}
	require.True(t, byName["here"].Enabled)
	require.True(t, byName["here"].Supported)
	require.Equal(t, "Entry for this architecture", byName["here"].Description)
	require.False(t, byName["elsewhere"].Enabled)
	require.False(t, byName["elsewhere"].Supported)
	// When
	resetTerminalMessages()
	text := newSidecarCommand(&CommonFlags{})
	text.SetArgs(append([]string{"list"}, selection...))
	require.NoError(t, text.ExecuteContext(testManagerContext(t)))
	// Then
	rendered := strings.Join(terminalOutputs, "")
	for _, want := range []string{
		"here", "enabled", "Entry for this architecture", runtime.GOARCH,
	} {
		require.Contains(t, rendered, want)
	}
}

//nolint:paralleltest // The embedded catalog is process-global.
func TestSidecarEnableReportsSupportedChoices(t *testing.T) {
	for _, name := range []string{"unknown-sidecar", "elsewhere"} {
		// Given
		withCatalog(t, architectureCatalog(t))
		directory := writeInitializedDeployment(t, "backend: local\nlocal: {}\n")
		deployment := config.NewDeploymentDir(directory)
		// When
		_, err := deploy.EnableSidecar(testManagerContext(t), deployment, name)
		// Then
		require.ErrorContains(t, err, "supported choices:")
		require.ErrorContains(t, err, name)
		require.ErrorContains(t, err, "here")
		require.NoFileExists(t, deployment.SidecarsPath())
	}
}
