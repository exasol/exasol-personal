// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
func TestSidecarFailureQueuesRestartGuidance(t *testing.T) {
	// Given
	resetTerminalMessages()
	t.Cleanup(resetTerminalMessages)
	cause := errors.Join(errors.New("another host failed"), deploy.ErrSidecarRestartRequired)
	// When
	err := installDeploymentFailure(cause)
	var stderr bytes.Buffer
	writeTerminalCallsToAction(&stderr, callsToActionVisible(false))
	// Then
	require.ErrorIs(t, err, deploy.ErrSidecarRestartRequired)
	require.NotContains(t, err.Error(), "exasol stop")
	require.Empty(t, terminalOutputs)
	require.Empty(t, terminalNotices)
	require.Equal(t, "\n"+sidecarRestartGuidance+"\n", stderr.String())
}

//nolint:paralleltest // Terminal queues are process-global.
func TestSidecarStartupFailureOffersStatus(t *testing.T) {
	for _, scenario := range []struct{ name, cause string }{
		{"occupied port", "127.0.0.1:18080: bind: address already in use"},
		{"unavailable image", "initializing source docker://caddy:2: reading manifest"},
	} {
		//nolint:paralleltest // Terminal queues are process-global.
		t.Run(scenario.name, func(t *testing.T) {
			// Given
			resetTerminalMessages()
			t.Cleanup(resetTerminalMessages)
			cause := sidecar.NewStartupError("caddy", scenario.cause, "exit status 126")
			// When
			err := installDeploymentFailure(cause)
			var stderr bytes.Buffer
			writeTerminalCallsToAction(&stderr, callsToActionVisible(false))
			// Then
			require.ErrorContains(t, err, "sidecar caddy")
			require.Contains(t, err.Error(), scenario.cause)
			require.Contains(t, stderr.String(), "exasol sidecar status caddy")
		})
	}
}

func TestSidecarEnableOffersNoWait(t *testing.T) {
	t.Parallel()
	// Given
	command := newSidecarCommand(&CommonFlags{})
	// When
	enable, _, err := command.Find([]string{"enable"})
	// Then
	require.NoError(t, err)
	require.NotNil(t, enable.Flags().Lookup("no-wait"))
}

//nolint:paralleltest // Terminal queues are process-global.
func TestSidecarHealthCallToAction(t *testing.T) {
	for _, fixture := range []struct{ health, want string }{
		{"starting", "Sidecar mcp is still starting. Run `exasol sidecar status mcp`"},
		{"failing", "Sidecar mcp is not serving. Address the probe failure"},
		{"serving", ""},
		{"", ""},
	} {
		// Given
		resetTerminalMessages()
		result := deploy.SidecarResult{Name: "mcp", Health: fixture.health}
		// When
		addSidecarHealthCallToAction(result)
		var stderr bytes.Buffer
		writeTerminalCallsToAction(&stderr, callsToActionVisible(false))
		// Then
		if fixture.want == "" {
			require.Empty(t, stderr.String())
		} else {
			require.Contains(t, stderr.String(), fixture.want)
		}
	}
	resetTerminalMessages()
}

//nolint:paralleltest // Terminal queues are process-global.
func TestSidecarRestartGuidanceFollowsOutputMode(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		// Given
		resetTerminalMessages()
		result := deploy.SidecarResult{
			Name: "example", Enabled: true, State: "stopped",
			RestartRequired: true, Endpoints: []deploy.SidecarEndpoint{},
		}
		// When
		err := renderSidecarResult(result, jsonOutput)
		var stdout, stderr bytes.Buffer
		writeTerminalMessages(terminalConfig{
			stdout: &stdout, stderr: &stderr,
			showCallsToAction: callsToActionVisible(jsonOutput),
		})
		// Then
		require.NoError(t, err)
		if jsonOutput {
			var decoded deploy.SidecarResult
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
			if !decoded.RestartRequired || !decoded.Enabled || decoded.Running ||
				stderr.Len() != 0 {
				t.Fatalf("JSON result: %+v, stderr=%q", decoded, stderr.String())
			}
		} else if !strings.Contains(stdout.String(), "State: stopped") ||
			!strings.Contains(
				stderr.String(),
				"exasol stop",
			) || !strings.Contains(stderr.String(), "exasol start") {
			t.Fatalf("text result: %q, %q", stdout.String(), stderr.String())
		}
	}
	resetTerminalMessages()
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
		Sidecars: map[string]sidecar.Definition{"saved": {Container: sidecar.Container{
			Image: "caddy:2",
			Env: []sidecar.EnvVar{{Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{
				SecretKeyRef: &sidecar.SecretKeyRef{Name: sidecar.DatabaseSecret, Key: "password"},
			}}},
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	command := newSidecarCommand(&CommonFlags{})
	command.SetArgs([]string{"enable", "saved", "--deployment-dir", directory, "--json"})
	// When
	err := command.ExecuteContext(testManagerContext(t))
	document, readErr := config.ReadSidecars(deployment)
	// Then
	if err != nil || readErr != nil || len(document.Sidecars["saved"].Container.Env) != 1 {
		t.Fatalf("saved definition: %+v, %v, %v", document, err, readErr)
	}
	var decoded deploy.SidecarResult
	require.NoError(t, json.Unmarshal([]byte(strings.Join(terminalOutputs, "")), &decoded))
	if !decoded.Enabled || decoded.Running || decoded.Name != "saved" {
		t.Fatalf("result: %+v", decoded)
	}
}

func TestSidecarTextIncludesEndpointsAndFailure(t *testing.T) {
	t.Parallel()
	// Given
	result := deploy.SidecarResult{
		Name: "example", Enabled: true,
		State: "exited", ExitCode: 1, Error: "startup failed",
		Reconciliation:     "pending",
		LastOperationError: "old cleanup failure",
		Endpoints:          []deploy.SidecarEndpoint{{IP: "::1", Port: 18080}},
	}
	// When
	text := formatSidecarResult(result)
	// Then
	for _, want := range []string{
		"example: enabled", "State: exited", "[::1]:18080", "Exit code: 1", "startup failed",
		"Reconciliation: pending", "Unresolved operation failure: old cleanup failure",
	} {
		require.Contains(t, text, want)
	}
}

func TestSidecarTextPrefersTheEndpointURL(t *testing.T) {
	t.Parallel()
	// Given
	result := deploy.SidecarResult{
		Name: "mcp", Enabled: true, Running: true,
		State: "running", Reconciliation: "complete",
		Endpoints: []deploy.SidecarEndpoint{
			{IP: "127.0.0.1", Port: 4896, URL: "http://127.0.0.1:4896/mcp"},
			{IP: "127.0.0.1", Port: 9090},
		},
	}
	// When
	text := formatSidecarResult(result)
	// Then
	require.Contains(t, text, "Endpoint: http://127.0.0.1:4896/mcp")
	require.Contains(t, text, "Endpoint: 127.0.0.1:9090")
	require.NotContains(t, text, "Endpoint: 127.0.0.1:4896\n")
}

func TestSidecarTextReportsHealth(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name   string
		result deploy.SidecarResult
		want   []string
		absent []string
	}{
		{
			"failing",
			deploy.SidecarResult{
				Name: "mcp", State: "running", Health: "failing",
				HealthError: "connection refused",
			},
			[]string{"Health: failing", "Probe failure: connection refused"},
			nil,
		},
		{
			"serving",
			deploy.SidecarResult{Name: "mcp", State: "running", Health: "serving"},
			[]string{"Health: serving"},
			[]string{"Probe failure"},
		},
		{
			"without probes",
			deploy.SidecarResult{Name: "mcp", State: "running"},
			nil,
			[]string{"Health", "Probe failure"},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			result := fixture.result
			// When
			text := formatSidecarResult(result)
			// Then
			for _, want := range fixture.want {
				require.Contains(t, text, want)
			}
			for _, absent := range fixture.absent {
				require.NotContains(t, text, absent)
			}
		})
	}
}

func TestSidecarTextReportsNothingToRemove(t *testing.T) {
	t.Parallel()
	// Given
	result := deploy.SidecarResult{Name: "custom", NothingToRemove: true}
	// When
	text := formatSidecarResult(result)
	// Then
	require.Equal(t, "Sidecar custom is already disabled; nothing to remove.", text)
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
		_, err := deploy.EnableSidecar(testManagerContext(t), deployment, name, true)
		// Then
		require.ErrorContains(t, err, "supported choices:")
		require.ErrorContains(t, err, name)
		require.ErrorContains(t, err, "here")
		require.NoFileExists(t, deployment.SidecarsPath())
	}
}

//nolint:paralleltest // The embedded catalog and terminal queues are process-global.
func TestSidecarListBrowsesCatalogWithoutDeployment(t *testing.T) {
	// Given
	withCatalog(t, architectureCatalog(t))
	t.Cleanup(resetTerminalMessages)
	resetTerminalMessages()
	// When
	command := newSidecarCommand(&CommonFlags{})
	command.SetArgs([]string{"list", "--json", "--deployment-dir", t.TempDir()})
	err := command.ExecuteContext(testManagerContext(t))
	// Then
	require.NoError(t, err)
	var listing []deploy.SidecarAvailability
	require.NoError(t, json.Unmarshal([]byte(strings.Join(terminalOutputs, "")), &listing))
	require.Len(t, listing, 2)
	for _, entry := range listing {
		require.False(t, entry.Enabled)
	}
}
