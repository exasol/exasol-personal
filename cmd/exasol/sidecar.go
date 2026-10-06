// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const sidecarRestartGuidance = "Run `exasol stop` followed by `exasol start` " +
	"to restart the deployment and start the enabled sidecars."

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
	var noDBPassword bool
	enable := newSidecarActionCommand(
		flags,
		"enable",
		"Enable a catalog sidecar",
		func(
			ctx context.Context, deployment config.DeploymentDir, name string,
		) (deploy.SidecarResult, error) {
			return deploy.EnableSidecar(ctx, deployment, name, noDBPassword)
		},
	)
	enable.Flags().BoolVar(&noDBPassword, "no-db-password", false,
		"Omit database password references from the saved sidecar definition")
	status := newSidecarActionCommand(flags, "status", "Show sidecar state and published endpoints",
		deploy.GetSidecarStatus)
	disable := newSidecarActionCommand(
		flags,
		"disable",
		"Disable a sidecar and remove its runtime resources",
		deploy.DisableSidecar,
	)
	for _, child := range []*cobra.Command{list, enable, status, disable} {
		child.SilenceUsage = true
		requireDefaultDeploymentCompatibility(child)
		requireInitializedDeploymentDir(child)
		if child != list {
			requireDeploymentFileLogging(child)
		}
		registerDeploymentDirFlag(child, flags)
		registerOutputFlags(child, flags)
		command.AddCommand(child)
	}

	return command
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
	if slices.ContainsFunc(
		result.Hosts,
		func(host deploy.SidecarHostResult) bool { return host.RestartRequired },
	) {
		addTerminalCallToAction(sidecarRestartGuidance)
	}
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
	lines := make([]string, 0, len(result.Hosts)+1)
	lines = append(lines, fmt.Sprintf("%s: %s", result.Name, enablement))
	for _, host := range result.Hosts {
		lines = append(lines, formatSidecarHost(host)...)
	}

	return strings.Join(lines, "\n")
}

func formatSidecarHost(result deploy.SidecarHostResult) []string {
	lines := []string{fmt.Sprintf("Host %s: %s", result.Name, result.State)}
	lines = append(lines, "Reconciliation: "+result.Reconciliation)
	if result.RestartRequired {
		lines = append(lines, "Deployment restart required.")
	}
	for _, endpoint := range result.Endpoints {
		lines = append(
			lines,
			"Endpoint: "+net.JoinHostPort(endpoint.IP, strconv.Itoa(endpoint.Port)),
		)
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

	return lines
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
