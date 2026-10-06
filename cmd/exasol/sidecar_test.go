// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
	enable, _, _ := command.Find([]string{"enable"})
	if enable.Flags().Lookup("no-db-password") == nil {
		t.Fatal("password opt-out missing")
	}
}

//nolint:paralleltest // Terminal queues are process-global.
func TestSidecarFailureGuidanceFollowsOutputMode(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		// Given
		resetTerminalMessages()
		t.Cleanup(resetTerminalMessages)
		cause := errors.Join(errors.New("another host failed"), deploy.ErrSidecarRestartRequired)
		// When
		err := installDeploymentFailure(cause)
		var stderr bytes.Buffer
		writeTerminalCallsToAction(&stderr, callsToActionVisible(jsonOutput), true)
		// Then
		require.ErrorIs(t, err, deploy.ErrSidecarRestartRequired)
		require.NotContains(t, err.Error(), "exasol stop")
		require.Empty(t, terminalOutputs)
		require.Empty(t, terminalNotices)
		if jsonOutput {
			require.Empty(t, stderr.String())
		} else {
			require.Equal(t, "\n"+sidecarRestartGuidance+"\n", stderr.String())
		}
	}
}

//nolint:paralleltest // Terminal queues are process-global.
func TestSidecarRestartGuidanceFollowsOutputMode(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		// Given
		resetTerminalMessages()
		result := deploy.SidecarResult{
			Name: "example", Enabled: true,
			Hosts: []deploy.SidecarHostResult{{
				Name: "local", State: "stopped",
				RestartRequired: true, Endpoints: []deploy.SidecarEndpoint{},
			}},
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
			if !decoded.Hosts[0].RestartRequired || !decoded.Enabled || decoded.Hosts[0].Running ||
				stderr.Len() != 0 {
				t.Fatalf("JSON result: %+v, stderr=%q", decoded, stderr.String())
			}
		} else if !strings.Contains(stdout.String(), "Host local: stopped") ||
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
func TestSidecarEnableCommandUsesSavedDefinitionAndPasswordOptOut(t *testing.T) {
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
	command.SetArgs(
		[]string{"enable", "saved", "--deployment-dir", directory, "--no-db-password", "--json"},
	)
	// When
	err := command.ExecuteContext(testManagerContext(t))
	document, readErr := config.ReadSidecars(deployment)
	// Then
	if err != nil || readErr != nil || len(document.Containers[0].Env) != 0 {
		t.Fatalf("saved opt-out: %+v, %v, %v", document, err, readErr)
	}
	var decoded deploy.SidecarResult
	require.NoError(t, json.Unmarshal([]byte(strings.Join(terminalOutputs, "")), &decoded))
	if !decoded.Enabled || decoded.Hosts[0].Running || decoded.Name != "saved" {
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
