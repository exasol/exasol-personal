// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

const (
	configCmdShortDesc = `Manage deployment configuration`
	configCmdLongDesc  = configCmdShortDesc + `

	Inspect, patch, or reset configuration parameters for the presets already
	initialized in the deployment directory. Configuration changes preserve local
	infrastructure state. Run exasol deploy after changing configuration to apply
	the changes to the deployment.
`
)

var configResetAll bool

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   configCmdShortDesc,
	Long:    configCmdLongDesc,
	GroupID: rootCmdGroupEssential,
}

var configGetCmd = &cobra.Command{
	Use:   "get [option-name...]",
	Short: "Show active deployment configuration",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		values, err := deploy.GetDeploymentConfiguration(
			cmd.Context(),
			commonFlags.Deployment(),
			args,
		)
		if err != nil {
			return fmt.Errorf("failed to read configuration: %w", err)
		}

		if commonFlags.OutputJson {
			return printConfigurationJSON(values)
		}
		safePrint(formatConfigurationValues(values))

		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Update deployment configuration options",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		infraVars := collectInfrastructureVariableOverrides(cmd)
		installVars := collectInstallationVariableOverrides(cmd)

		values, err := deploy.SetDeploymentConfiguration(
			cmd.Context(),
			infraVars,
			installVars,
			commonFlags.Deployment(),
		)
		if err != nil {
			return fmt.Errorf("configuration failed: %w", err)
		}

		addTerminalNotice(formatConfigurationChangedNotice(values))

		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset [option-name...]",
	Short: "Reset deployment configuration options to defaults",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		values, err := deploy.ResetDeploymentConfiguration(
			cmd.Context(),
			commonFlags.Deployment(),
			args,
			configResetAll,
		)
		if err != nil {
			return fmt.Errorf("configuration reset failed: %w", err)
		}

		addTerminalNotice(formatConfigurationChangedNotice(values))

		return nil
	},
}

// nolint: gochecknoinits
func init() {
	for _, cmd := range []*cobra.Command{configCmd, configGetCmd, configSetCmd, configResetCmd} {
		requireMinorVersionCompatibility(cmd, CurrentLauncherVersion)
		requireInitializedDeploymentDir(cmd)
		requireDeploymentFileLogging(cmd)
	}
	registerDeploymentDirFlag(configGetCmd, commonFlags)
	registerDeploymentDirFlag(configSetCmd, commonFlags)
	registerDeploymentDirFlag(configResetCmd, commonFlags)

	registerOutputFlags(configGetCmd, commonFlags)
	configResetCmd.Flags().BoolVar(&configResetAll, "all", false, "Reset all options to defaults")

	configCmd.AddCommand(configGetCmd, configSetCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}

func printConfigurationJSON(values []deploy.DeploymentConfigValue) error {
	result := map[string]any{}
	for _, value := range values {
		result[value.Name] = value.Value
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	safePrint(string(data) + "\n")

	return nil
}

func formatConfigurationChangedNotice(values []deploy.DeploymentConfigValue) string {
	return strings.TrimRight(formatConfigurationValues(values), "\n") +
		"\nconfiguration updated locally; run `exasol deploy` to apply these changes"
}

func formatConfigurationValues(values []deploy.DeploymentConfigValue) string {
	if len(values) == 0 {
		return "Active configuration:\n  (no configurable options)\n"
	}

	var builder strings.Builder
	_, _ = builder.WriteString("Active configuration:\n")
	for _, value := range values {
		_, _ = builder.WriteString("  ")
		_, _ = builder.WriteString(value.Name)
		_, _ = builder.WriteString(" = ")
		_, _ = builder.WriteString(formatConfigurationScalar(value.Value))
		_, _ = builder.WriteString("\n")
	}

	return builder.String()
}

func formatConfigurationScalar(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		if value == nil {
			return ""
		}

		return fmt.Sprint(value)
	}
}

func hasConfigurationVariableOverrides(
	infraVars map[string]string,
	installVars map[string]string,
) bool {
	return len(infraVars) > 0 || len(installVars) > 0
}

func applyConfigurationPatchIfProvided(
	cmd *cobra.Command,
	infraVars map[string]string,
	installVars map[string]string,
) ([]deploy.DeploymentConfigValue, bool, error) {
	if !hasConfigurationVariableOverrides(infraVars, installVars) {
		return nil, false, nil
	}

	values, err := deploy.SetDeploymentConfiguration(
		cmd.Context(),
		infraVars,
		installVars,
		commonFlags.Deployment(),
	)
	if err != nil {
		return nil, false, fmt.Errorf("configuration failed: %w", err)
	}

	return values, true, nil
}
