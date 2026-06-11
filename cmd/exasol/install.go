// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

var installCmdShortDesc = `Initialize, apply configuration, and deploy Exasol in one step`

var installCmdLongDesc = installCmdShortDesc + `

	Initializes the deployment directory or applies supplied configuration, prepares infrastructure,
	and installs Exasol.
	Rerunning install with the same presets is safe for retrying failed deployments.
	To switch presets, run exasol destroy --remove first, or exasol remove if the
	deployment resources are already gone.` +
	deploymentDirectoryResolutionHelp + presetSelectionHelp

var installCmd = &cobra.Command{
	Use:   "install <infra preset name-or-path> [install preset name-or-path]",
	Short: installCmdShortDesc,
	Long:  installCmdLongDesc,
	Example: "  exasol install " + presets.DefaultInfrastructure + "\n" +
		"  exasol install " + presets.DefaultInfrastructure + " " + presets.DefaultInstallation,
	Args:    cobra.RangeArgs(minPresetArgs, maxPresetArgs),
	GroupID: rootCmdGroupEssential,
}

// nolint: gochecknoinits
func init() {
	// Install creates (init) and then operates on (deploy) the deployment directory.
	requireDeploymentFileLogging(installCmd)

	installCmd.Long = strings.TrimRight(installCmd.Long, "\n") +
		"\n\t" + presetNamesForHelp(presets.PresetTypeInfrastructure,
		presets.ListEmbeddedInfrastructuresPresets()) +
		"\n\t" + presetNamesForHelp(presets.PresetTypeInstallation,
		presets.ListEmbeddedInstallationsPresets())

	if matrix := embeddedPresetCompatibilityMatrix(); matrix != "" {
		installCmd.Long += "\n\n\t" + strings.ReplaceAll(matrix, "\n", "\n\t")
	}

	// Run initialization
	installCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		deployment := commonFlags.Deployment()
		wasInitialized, err := config.HasExasolPersonalStateFile(deployment)
		if err != nil {
			return err
		}
		infraVars := collectInfrastructureVariableOverrides(cmd)
		installVars := collectInstallationVariableOverrides(cmd)
		infraPreset := presetRefFromArg(args[0])
		installPreset, err := resolveInstallationPresetRef(args, 1, infraPreset)
		if err != nil {
			return err
		}

		if wasInitialized {
			if err := prepareInitializedInstall(
				cmd,
				deployment,
				infraPreset,
				installPreset,
				infraVars,
				installVars,
			); err != nil {
				return fmt.Errorf("initialization failed: %w", err)
			}
		} else {
			if err := deploy.InitDeployment(
				cmd.Context(),
				infraPreset,
				installPreset,
				infraVars,
				installVars,
				deployment,
				!commonFlags.NoLauncherVersionCheck,
				CurrentLauncherVersion,
			); err != nil {
				return fmt.Errorf("initialization failed: %w", err)
			}
		}

		if err := setupDeploymentLogSession(cmd, deployment); err != nil {
			return err
		}
		addTerminalNotice(deploy.EulaNoticeText)

		return nil
	}

	// Perform deployment after initialization completes.
	installCmd.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error {
		deployment := commonFlags.Deployment()
		if err := deploy.Deploy(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
			deploy.DeployOptions{
				UpdateDependencyLockfile: commonFlags.DeployTofuUpdateLockfile,
			},
		); err != nil {
			return fmt.Errorf("deployment failed: %w", err)
		}

		err := addConnectionInstructionsTerminalOutput(deployment)
		if err != nil {
			return fmt.Errorf("failed to print deployment info: %w", err)
		}

		return nil
	}

	// Make "install" runnable without subcommands; no-op RunE prevents usage output.
	// init runs in PreRunE, deploy runs in PersistentPostRunE
	installCmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }

	requireMinorVersionCompatibility(installCmd, CurrentLauncherVersion)
	registerInitFlags(installCmd, commonFlags)
	registerDeploymentDirFlag(installCmd, commonFlags)
	registerVerboseFlag(installCmd, commonFlags)
	registerDeployFlags(installCmd, commonFlags)
	rootCmd.AddCommand(installCmd)
}

func prepareInitializedInstall(
	cmd *cobra.Command,
	deployment config.DeploymentDir,
	infraPreset deploy.PresetRef,
	installPreset deploy.PresetRef,
	infraVars map[string]string,
	installVars map[string]string,
) error {
	if err := ensureRequestedPresetsMatchInitializedDeployment(
		deployment,
		infraPreset,
		installPreset,
	); err != nil {
		return err
	}
	_, _, err := applyConfigurationPatchIfProvided(cmd, infraVars, installVars)

	return err
}
