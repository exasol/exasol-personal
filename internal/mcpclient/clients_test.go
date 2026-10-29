// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package mcpclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testEndpoint = "http://127.0.0.1:14896/mcp"

func TestEveryClientRegistersTheEndpoint(t *testing.T) {
	t.Parallel()
	require.Equal(t, []string{
		"claude-code", "claude-desktop", "codex", "cursor",
		"vscode", "gemini", "opencode", "continue",
	}, IDs())
	for _, client := range Clients() {
		// When
		instructions := client.Instructions(testEndpoint)
		// Then
		require.Equal(t, client.ID, instructions.Client)
		require.Equal(t, testEndpoint, instructions.URL)
		require.NotEqual(t, instructions.Command == "", instructions.File == "", client.ID)
		require.Contains(t, instructions.Command+instructions.Content, testEndpoint, client.ID)
		require.Contains(t, instructions.Command+instructions.Content, ServerName, client.ID)
	}
}

func TestClientInstructions(t *testing.T) {
	t.Parallel()
	for clientID, expected := range map[string]string{
		"claude-code": "claude mcp add --transport http --scope user exasol-personal " +
			testEndpoint,
		"codex":    "codex mcp add exasol-personal --url " + testEndpoint,
		"gemini":   "gemini mcp add --transport http --scope user exasol-personal " + testEndpoint,
		"opencode": "opencode mcp add exasol-personal --url " + testEndpoint,
		"vscode": `code --add-mcp '{"name":"exasol-personal","type":"http","url":"` +
			testEndpoint + `"}'`,
	} {
		client, found := Lookup(clientID)
		require.True(t, found)
		require.Equal(t, expected, client.Instructions(testEndpoint).Command)
	}
	cursor, _ := Lookup("cursor")
	require.Equal(t, "~/.cursor/mcp.json", cursor.Instructions(testEndpoint).File)
	require.JSONEq(t,
		`{"mcpServers":{"exasol-personal":{"url":"`+testEndpoint+`"}}}`,
		cursor.Instructions(testEndpoint).Content)
	continueClient, _ := Lookup("continue")
	require.Contains(t, continueClient.Instructions(testEndpoint).Content,
		"type: streamable-http\n    url: "+testEndpoint)
}

func TestClaudeDesktopBridgeAllowsPlainHTTPOnly(t *testing.T) {
	t.Parallel()
	client, _ := Lookup("claude-desktop")
	require.Contains(t, client.Instructions(testEndpoint).Content, "--allow-http")
	secure := client.Instructions("https://127.0.0.1:14896/mcp").Content
	require.NotContains(t, secure, "--allow-http")
}

func TestLookupRejectsUnknownClients(t *testing.T) {
	t.Parallel()
	_, found := Lookup("unknown")
	require.False(t, found)
}
