// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/mcpclient"
	"github.com/spf13/cobra"
)

const mcpCmdShortDesc = "Connect AI agents to the database through the Exasol MCP server"

const mcpCmdLongDesc = mcpCmdShortDesc + `

The MCP server runs as the "mcp" sidecar of a local deployment. These commands
act on that sidecar like the matching "exasol sidecar" commands.

Example usage:
    exasol mcp enable
    exasol mcp status
    exasol mcp instructions claude-code
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
	// Listing clients answers before a deployment exists.
	addSidecarSubcommand(command, newMCPInstructionsCommand(flags), flags, false)

	return command
}

const (
	mcpEnableGuidance       = "Run `exasol mcp enable` to enable the MCP server."
	mcpInstructionsGuidance = "Run `exasol mcp instructions <client>` " +
		"to show its setup instructions."
	mcpInstallGuidance = "Run `exasol install local` to create a new local deployment."
	mcpSelectGuidance  = "Run `exasol mcp instructions %s -d NAME` " +
		"to use an existing deployment."
)

func newMCPInstructionsCommand(flags *CommonFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "instructions [client]",
		Short: "Show client setup instructions",
		Long: "Show client setup instructions\n\n" +
			"Without a client, list the clients with setup instructions. With one, print\n" +
			"the command or configuration file entry that registers the MCP server's\n" +
			"endpoint with it. The launcher does not change the client's configuration.\n" +
			"Other MCP clients that support Streamable HTTP can use the endpoint that\n" +
			"\"exasol mcp status\" reports.\n\n" +
			"Instructions are available for: " + strings.Join(mcpclient.IDs(), ", "),
		Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 0 {
				return renderMCPClients(flags.OutputJson)
			}
			client, found := mcpclient.Lookup(args[0])
			if !found {
				return fmt.Errorf(
					"no setup instructions for MCP client %q; instructions are available for: %s",
					args[0], strings.Join(mcpclient.IDs(), ", "))
			}
			if err := requireMCPDeployment(flags.Deployment(), client.ID); err != nil {
				return err
			}
			endpoint, err := deploy.MCPServerURL(command.Context(), flags.Deployment())
			if errors.Is(err, deploy.ErrMCPServerNotEnabled) {
				addTerminalCallToAction(mcpEnableGuidance)
			}
			if err != nil {
				return err
			}
			instructions := client.Instructions(endpoint)
			if flags.OutputJson {
				return addJSONTerminalOutput(instructions)
			}
			addTerminalOutput(formatMCPInstructions(instructions))

			return nil
		},
	}
}

// requireMCPDeployment explains that client instructions need a deployment,
// since the listing alone answers without one.
func requireMCPDeployment(deployment config.DeploymentDir, client string) error {
	initialized, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil || initialized {
		return err
	}
	addTerminalCallToAction(mcpInstallGuidance)
	addTerminalCallToAction(fmt.Sprintf(mcpSelectGuidance, client))

	return fmt.Errorf(
		"no initialized deployment was found in %q.\n"+
			"Client setup instructions use the selected deployment's MCP endpoint",
		deployment.Root(),
	)
}

func renderMCPClients(jsonOutput bool) error {
	clients := mcpclient.Clients()
	if jsonOutput {
		return addJSONTerminalOutput(clients)
	}
	lines := []string{"Setup instructions are available for these MCP clients:"}
	for _, client := range clients {
		lines = append(lines, fmt.Sprintf("  %-16s %s", client.ID, client.Name))
	}
	addTerminalOutput(strings.Join(lines, "\n"))
	addTerminalCallToAction(mcpInstructionsGuidance)

	return nil
}

func formatMCPInstructions(instructions mcpclient.Instructions) string {
	var lines []string
	if instructions.Command != "" {
		lines = append(lines,
			"Run this command to register the MCP server with "+instructions.Name+":",
			"",
			"  "+instructions.Command,
		)
	} else {
		lines = append(lines,
			"Add this entry to "+instructions.File+" for "+instructions.Name+
				", keeping any servers already there:",
			"",
			strings.TrimRight(instructions.Content, "\n"),
		)
	}
	if instructions.Note != "" {
		lines = append(lines, "", instructions.Note)
	}

	return strings.Join(lines, "\n")
}
