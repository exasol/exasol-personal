// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package mcpclient describes how MCP clients register a Streamable HTTP
// server.
package mcpclient

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
)

// ServerName differs from the starter kit's "exasol", so a client can hold both.
const ServerName = "exasol-personal"

type Client struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	instructions func(endpoint string) Instructions
}

// Instructions carry either a command to run or file content to merge.
type Instructions struct {
	Client  string `json:"client"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Command string `json:"command,omitempty"`
	File    string `json:"file,omitempty"`
	Content string `json:"content,omitempty"`
	Note    string `json:"note,omitempty"`
}

func Clients() []Client {
	return []Client{
		{ID: "claude-code", Name: "Claude Code", instructions: func(endpoint string) Instructions {
			return Instructions{Command: fmt.Sprintf(
				"claude mcp add --transport http --scope user %s %s", ServerName, endpoint,
			)}
		}},
		{ID: "claude-desktop", Name: "Claude Desktop", instructions: claudeDesktop},
		{ID: "codex", Name: "Codex", instructions: func(endpoint string) Instructions {
			return Instructions{Command: fmt.Sprintf(
				"codex mcp add %s --url %s", ServerName, endpoint,
			)}
		}},
		{ID: "cursor", Name: "Cursor", instructions: func(endpoint string) Instructions {
			return Instructions{
				File:    "~/.cursor/mcp.json",
				Content: serversContent(map[string]any{"url": endpoint}),
			}
		}},
		{ID: "vscode", Name: "VS Code (GitHub Copilot)", instructions: vscode},
		{ID: "gemini", Name: "Gemini CLI", instructions: func(endpoint string) Instructions {
			return Instructions{Command: fmt.Sprintf(
				"gemini mcp add --transport http --scope user %s %s", ServerName, endpoint,
			)}
		}},
		{ID: "opencode", Name: "OpenCode", instructions: func(endpoint string) Instructions {
			return Instructions{Command: fmt.Sprintf(
				"opencode mcp add %s --url %s", ServerName, endpoint,
			)}
		}},
		{ID: "continue", Name: "Continue", instructions: func(endpoint string) Instructions {
			return Instructions{
				File: "~/.continue/mcpServers/" + ServerName + ".yaml",
				Content: fmt.Sprintf(
					"name: %[1]s\nversion: 0.0.1\nschema: v1\nmcpServers:\n"+
						"  - name: %[1]s\n    type: streamable-http\n    url: %[2]s\n",
					ServerName, endpoint,
				),
			}
		}},
	}
}

// Claude Desktop reads only stdio servers from its file, so a bridge relays
// to the HTTP endpoint.
func claudeDesktop(endpoint string) Instructions {
	args := []string{"-y", "mcp-remote", endpoint}
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Scheme == "http" {
		args = append(args, "--allow-http")
	}

	return Instructions{
		File:    "claude_desktop_config.json",
		Content: serversContent(map[string]any{"command": "npx", "args": args}),
		Note: "Open it through Settings > Developer > Edit Config. " +
			"The bridge requires Node.js.",
	}
}

func vscode(endpoint string) Instructions {
	definition := fmt.Sprintf(`{"name":%q,"type":"http","url":%q}`, ServerName, endpoint)

	return Instructions{Command: fmt.Sprintf("code --add-mcp '%s'", definition)}
}

// serversContent builds the mcpServers document the JSON-configured clients share.
func serversContent(server map[string]any) string {
	document := map[string]map[string]map[string]any{
		"mcpServers": {ServerName: server},
	}
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		panic(err)
	}

	return string(content) + "\n"
}

func Lookup(id string) (Client, bool) {
	clients := Clients()
	index := slices.IndexFunc(clients, func(client Client) bool { return client.ID == id })
	if index < 0 {
		return Client{}, false
	}

	return clients[index], true
}

func IDs() []string {
	clients := Clients()
	ids := make([]string, 0, len(clients))
	for _, client := range clients {
		ids = append(ids, client.ID)
	}

	return ids
}

func (client Client) Instructions(endpoint string) Instructions {
	instructions := client.instructions(endpoint)
	instructions.Client, instructions.Name, instructions.URL = client.ID, client.Name, endpoint

	return instructions
}
