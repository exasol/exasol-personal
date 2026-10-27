// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const sidecarRestartGuidance = "Run `exasol stop` followed by `exasol start` " +
	"to restart the deployment and start the enabled sidecars."

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
	enable := newSidecarEnableCommand(flags)
	status := newSidecarStatusCommand(flags)
	disable := newSidecarDisableCommand(flags)
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

func newSidecarEnableCommand(flags *CommonFlags) *cobra.Command {
	var noWait bool
	command := newSidecarActionCommand(
		flags,
		"enable",
		"Enable a catalog sidecar and wait until it serves",
		func(
			ctx context.Context, deployment config.DeploymentDir, name string,
		) (deploy.SidecarResult, error) {
			result, err := deploy.EnableSidecar(ctx, deployment, name, !noWait)
			if err == nil {
				addSidecarHealthCallToAction(result)
			}

			return result, err
		},
	)
	command.Flags().BoolVar(
		&noWait, "no-wait", false,
		"Return without waiting for a starting sidecar to serve",
	)

	return command
}

const (
	sidecarStartingGuidance = "Sidecar %[1]s is still starting. " +
		"Run `exasol sidecar status %[1]s` to check whether it is serving."
	sidecarFailingGuidance = "Sidecar %[1]s is not serving. Address the probe failure, " +
		"then run `exasol sidecar status %[1]s` to check it again."
)

func addSidecarHealthCallToAction(result deploy.SidecarResult) {
	switch result.Health {
	case "starting":
		addTerminalCallToAction(fmt.Sprintf(sidecarStartingGuidance, result.Name))
	case "failing":
		addTerminalCallToAction(fmt.Sprintf(sidecarFailingGuidance, result.Name))
	default:
	}
}

func newSidecarDisableCommand(flags *CommonFlags) *cobra.Command {
	var remove bool
	command := newSidecarActionCommand(
		flags,
		"disable",
		"Disable a sidecar and remove its runtime resources",
		func(
			ctx context.Context, deployment config.DeploymentDir, name string,
		) (deploy.SidecarResult, error) {
			if remove {
				return deploy.RemoveSidecar(ctx, deployment, name)
			}

			return deploy.DisableSidecar(ctx, deployment, name)
		},
	)
	command.Flags().BoolVar(
		&remove, "remove", false,
		"Also delete the saved definition",
	)

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
					addDeploymentRecoveryCallToActionFor(err, flags.Selection())

					return err
				}

				return renderSidecarResult(result, flags.OutputJson)
			}
			results, err := deploy.GetAllSidecarStatus(command.Context(), flags.Deployment())
			if err != nil {
				addDeploymentRecoveryCallToActionFor(err, flags.Selection())

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
		if result.RestartRequired {
			addTerminalCallToAction(sidecarRestartGuidance)
		}
		addSidecarDeferredCallToAction(result)
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
				addDeploymentRecoveryCallToActionFor(err, flags.Selection())
				return err
			}

			return renderSidecarResult(result, flags.OutputJson)
		},
	}
}

const sidecarDeferredGuidance = "Sidecar %s will run after the next " +
	"`exasol deploy` or `exasol start`."

// addSidecarDeferredCallToAction answers the question enable leaves open on a
// deployment whose database is not running.
func addSidecarDeferredCallToAction(result deploy.SidecarResult) {
	if result.Enabled && !result.Running {
		addTerminalCallToAction(fmt.Sprintf(sidecarDeferredGuidance, result.Name))
	}
}

func renderSidecarResult(result deploy.SidecarResult, jsonOutput bool) error {
	if result.RestartRequired {
		addTerminalCallToAction(sidecarRestartGuidance)
	}
	addSidecarDeferredCallToAction(result)
	if jsonOutput {
		return addJSONTerminalOutput(result)
	}
	addTerminalOutput(formatSidecarResult(result))

	return nil
}

func formatSidecarResult(result deploy.SidecarResult) string {
	if result.NothingToRemove {
		return fmt.Sprintf("Sidecar %s is already disabled; nothing to remove.", result.Name)
	}
	enablement := "disabled"
	if result.Enabled {
		enablement = "enabled"
	}
	lines := []string{
		fmt.Sprintf("%s: %s", result.Name, enablement),
		"State: " + result.State,
		"Reconciliation: " + result.Reconciliation,
	}
	if result.Health != "" {
		lines = append(lines, "Health: "+result.Health)
	}
	if result.HealthError != "" {
		lines = append(lines, "Probe failure: "+result.HealthError)
	}
	for _, endpoint := range result.Endpoints {
		address := endpoint.URL
		if address == "" {
			address = net.JoinHostPort(endpoint.IP, strconv.Itoa(endpoint.Port))
		}
		lines = append(lines, "Endpoint: "+address)
	}
	if result.State == "exited" {
		lines = append(lines, fmt.Sprintf("Exit code: %d", result.ExitCode))
	}
	if result.Error != "" {
		lines = append(lines, "Failure: "+result.Error)
	}
	if result.LastOperationError != "" {
		lines = append(lines, "Unresolved operation failure: "+result.LastOperationError)
	}

	return strings.Join(lines, "\n")
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
