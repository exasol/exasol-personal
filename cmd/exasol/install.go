// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

var installCmdShortDesc = `Initialize and deploy Exasol in one step`

var installCmdLongDesc = installCmdShortDesc + `

	Initializes the deployment directory, prepares infrastructure, and installs Exasol.` +
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
		deployment := commonFlags.Deployment()
		infraVars := collectInfrastructureVariableOverrides(cmd)
		installVars := collectInstallationVariableOverrides(cmd)
		infraPreset := presetRefFromArg(args[0])
		installPreset, err := resolveInstallationPresetRef(args, 1, infraPreset)
		if err != nil {
			return err
		}

		safePrint(deploy.EulaNoticeText)

		// Run initialization
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

		return setupDeploymentLogSession(cmd, deployment)
	}

	// Perform deployment after initialization completes.
	installCmd.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error {
		deployment := commonFlags.Deployment()
		lockfileMode := deploy.TofuLockfileReadonly
		if commonFlags.DeployTofuUpdateLockfile {
			lockfileMode = deploy.TofuLockfileUpdate
		}
		if err := deploy.Deploy(
			cmd.Context(),
			deployment,
			commonFlags.DeployVerbose,
			lockfileMode,
		); err != nil {
			return fmt.Errorf("deployment failed: %w", err)
		}

		err := printConnectionInstructionsFromFile(deployment, os.Stdout)
		if err != nil {
			return fmt.Errorf("failed to print deployment info: %w", err)
		}

		return nil
	}

	// Make "install" runnable without subcommands; no-op RunE prevents usage output.
	// init runs in PreRunE, deploy runs in PersistentPostRunE
	installCmd.RunE = func(_ *cobra.Command, _ []string) error { return nil }

	requireMinorVersionCompatibility(installCmd, CurrentLauncherVersion)
	registerDeploymentDirFlag(installCmd, commonFlags)
	registerVerboseFlag(installCmd, commonFlags)
	registerDeployFlags(installCmd, commonFlags)
	rootCmd.AddCommand(installCmd)
}
