// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const mcpCmdShortDesc = "Connect AI agents to the database through the Exasol MCP server"

const mcpCmdLongDesc = mcpCmdShortDesc + `

The MCP server runs as the "mcp" sidecar of a local deployment. These commands
act on that sidecar like the matching "exasol sidecar" commands.

Example usage:
    exasol mcp enable
    exasol mcp status
`

// nolint: gochecknoinits
func init() {
	if deploy.CatalogSupports(deploy.MCPSidecarName) {
		rootCmd.AddCommand(newMCPCommand(commonFlags))
	}
}

func newMCPCommand(flags *CommonFlags) *cobra.Command {
	command := &cobra.Command{
		Use: "mcp", Short: mcpCmdShortDesc, Long: mcpCmdLongDesc,
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	enable := newSidecarEnableCommand(flags, deploy.MCPSidecarName)
	enable.Short = "Enable the MCP server and wait until it serves"
	status := newSidecarStatusCommand(flags, deploy.MCPSidecarName)
	status.Short = "Show the MCP server's state and endpoint"
	disable := newSidecarDisableCommand(flags, deploy.MCPSidecarName)
	disable.Short = "Disable the MCP server and remove its runtime resources"
	for _, child := range []*cobra.Command{enable, status, disable} {
		addSidecarSubcommand(command, child, flags, true)
	}

	return command
}
