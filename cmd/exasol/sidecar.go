// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(newSidecarCommand(commonFlags))
}

func newSidecarCommand(flags *CommonFlags) *cobra.Command {
	command := &cobra.Command{
		Use: "sidecar", Short: "Manage deployment sidecars from the built-in catalog",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error { return command.Help() },
	}
	list := &cobra.Command{
		Use: "list", Short: "List catalog sidecars and deployment enablement", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deploy.ListSidecars(command.Context(), flags.Deployment())
			if err != nil {
				return err
			}
			if flags.OutputJson {
				return addJSONTerminalOutput(result)
			}
			addTerminalOutput(formatSidecarList(result))

			return nil
		},
	}
	enable := newSidecarActionCommand(
		flags, "enable", "Enable a catalog sidecar", deploy.EnableSidecar,
	)
	status := newSidecarStatusCommand(flags)
	disable := newSidecarActionCommand(
		flags,
		"disable",
		"Disable a sidecar and remove its runtime resources",
		deploy.DisableSidecar,
	)
	for _, child := range []*cobra.Command{list, enable, status, disable} {
		child.SilenceUsage = true
		requireDefaultDeploymentCompatibility(child)
		// Listing serves the catalog itself, so it answers before a deployment exists.
		if child != list {
			requireInitializedDeploymentDir(child)
			requireDeploymentFileLogging(child)
		}
		registerDeploymentDirFlag(child, flags)
		registerOutputFlags(child, flags)
		command.AddCommand(child)
	}

	return command
}

// newSidecarStatusCommand reports one sidecar, or every enabled one.
func newSidecarStatusCommand(flags *CommonFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status [name]",
		Short: "Show sidecar state and published endpoints",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 1 {
				result, err := deploy.GetSidecarStatus(
					command.Context(), flags.Deployment(), args[0],
				)
				if err != nil {
					addDeploymentRecoveryCallToAction(err)

					return err
				}

				return renderSidecarResult(result, flags.OutputJson)
			}
			results, err := deploy.GetAllSidecarStatus(command.Context(), flags.Deployment())
			if err != nil {
				addDeploymentRecoveryCallToAction(err)

				return err
			}

			return renderSidecarResults(results, flags.OutputJson)
		},
	}
}

func renderSidecarResults(results []deploy.SidecarResult, jsonOutput bool) error {
	if jsonOutput {
		return addJSONTerminalOutput(results)
	}
	if len(results) == 0 {
		addTerminalOutput("No sidecars are enabled for this deployment.")

		return nil
	}
	blocks := make([]string, 0, len(results))
	for _, result := range results {
		blocks = append(blocks, formatSidecarResult(result))
	}
	addTerminalOutput(strings.Join(blocks, "\n\n"))

	return nil
}

func newSidecarActionCommand(
	flags *CommonFlags,
	name, description string,
	action func(context.Context, config.DeploymentDir, string) (deploy.SidecarResult, error),
) *cobra.Command {
	return &cobra.Command{
		Use: name + " <name>", Short: description, Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := action(command.Context(), flags.Deployment(), args[0])
			if err != nil {
				addDeploymentRecoveryCallToAction(err)
				return err
			}

			return renderSidecarResult(result, flags.OutputJson)
		},
	}
}

func renderSidecarResult(result deploy.SidecarResult, jsonOutput bool) error {
	if jsonOutput {
		return addJSONTerminalOutput(result)
	}
	addTerminalOutput(formatSidecarResult(result))

	return nil
}

func formatSidecarResult(result deploy.SidecarResult) string {
	enablement := "disabled"
	if result.Enabled {
		enablement = "enabled"
	}

	return fmt.Sprintf("%s: %s", result.Name, enablement)
}

func formatSidecarList(entries []deploy.SidecarAvailability) string {
	if len(entries) == 0 {
		return "No sidecars are available in this launcher's catalog."
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		state := "disabled"
		if entry.Enabled {
			state = "enabled"
		}
		if !entry.Supported {
			state += ", unsupported architecture"
		}
		lines = append(lines, fmt.Sprintf("%s (%s): %s [%s]", entry.Name,
			strings.Join(entry.Architectures, ", "), entry.Description, state))
	}

	return strings.Join(lines, "\n")
}
