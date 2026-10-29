// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/mcpclient"
	"github.com/stretchr/testify/require"
)

func TestMCPCommandBindsSidecarSubcommandsToTheMCPServer(t *testing.T) {
	t.Parallel()
	// Given
	command := newMCPCommand(&CommonFlags{})
	// When / Then
	for _, name := range []string{"enable", "status", "disable"} {
		child, _, err := command.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, child.Name())
		for _, flag := range []string{"deployment", "deployment-dir", "json"} {
			require.NotNil(t, child.Flags().Lookup(flag), "%s missing --%s", name, flag)
		}
		require.NoError(t, child.Args(child, nil))
		require.Error(t, child.Args(child, []string{"mcp"}))
		require.True(t, deploymentFileLoggingIsRequired(child))
	}
	enable, _, _ := command.Find([]string{"enable"})
	require.NotNil(t, enable.Flags().Lookup("no-wait"))
	disable, _, _ := command.Find([]string{"disable"})
	require.NotNil(t, disable.Flags().Lookup("remove"))
}

func TestFormatMCPInstructions(t *testing.T) {
	t.Parallel()
	// Given
	command := mcpclient.Instructions{Name: "Codex", Command: "codex mcp add x --url u"}
	file := mcpclient.Instructions{
		Name: "Cursor", File: "~/.cursor/mcp.json", Content: "{}\n", Note: "Restart it.",
	}
	// When / Then
	require.Equal(t,
		"Run this command to register the MCP server with Codex:\n\n  codex mcp add x --url u",
		formatMCPInstructions(command))
	require.Equal(t,
		"Add this entry to ~/.cursor/mcp.json for Cursor, keeping any servers already there:"+
			"\n\n{}\n\nRestart it.",
		formatMCPInstructions(file))
}

func TestMCPInstructionsRejectsUnknownClients(t *testing.T) {
	t.Parallel()
	// Given
	command := newMCPInstructionsCommand(&CommonFlags{})
	// When
	err := command.RunE(command, []string{"unknown"})
	// Then
	require.ErrorContains(t, err, `no setup instructions for MCP client "unknown"`)
	require.ErrorContains(t, err, "instructions are available for: claude-code")
}

func TestRequireMCPDeploymentNamesTheUninitializedDirectory(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	// When
	err := requireMCPDeployment(deployment, "codex")
	// Then
	require.ErrorContains(t, err, "no initialized deployment was found in")
	require.ErrorContains(t, err, deployment.Root())
	require.ErrorContains(t, err, "use the selected deployment's MCP endpoint")
}

func TestRequireMCPDeploymentAcceptsAnInitializedDirectory(t *testing.T) {
	t.Parallel()
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, os.WriteFile(deployment.ExasolPersonalStatePath(), []byte("{}"), 0o600))
	// When / Then
	require.NoError(t, requireMCPDeployment(deployment, "codex"))
}
