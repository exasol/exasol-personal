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

	Extracts the specified infrastructure and installation presets into the deployment directory.

	Preset arguments:
	  - The first argument selects the infrastructure preset (required).
	  - The optional second argument selects the installation preset.

	Each argument can be either an embedded preset name (e.g. "aws") or a preset directory path.
	To force path selection, pass a path-like value such as "./my-preset" or "/abs/path/to/preset".

	Tip: use "exasol presets" to discover and export presets.
	`

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

	// Run initialization
	installCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		infraVars := collectInfrastructureVariableOverrides(cmd)
		installVars := collectInstallationVariableOverrides(cmd)
		infraPreset := presetRefFromArg(args[0])
		installPreset := defaultedPresetRefFromOptionalArg(
			args, 1, defaultInstallationPresetRefForInfra(infraPreset))

		safePrint(deploy.EulaNoticeText)

		// Run initialization
		if err := deploy.InitDeployment(
			cmd.Context(),
			infraPreset,
			installPreset,
			infraVars,
			installVars,
			commonFlags.DeploymentDir,
			!commonFlags.NoLauncherVersionCheck,
			CurrentLauncherVersion,
		); err != nil {
			return fmt.Errorf("initialization failed: %w", err)
		}

		return setupDeploymentLogSession(cmd, commonFlags.DeploymentDir)
	}

	// Perform deployment after initialization completes.
	installCmd.PersistentPostRunE = func(cmd *cobra.Command, _ []string) error {
		lockfileMode := deploy.TofuLockfileReadonly
		if commonFlags.DeployTofuUpdateLockfile {
			lockfileMode = deploy.TofuLockfileUpdate
		}
		if err := deploy.Deploy(
			cmd.Context(),
			commonFlags.DeploymentDir,
			commonFlags.DeployVerbose,
			lockfileMode,
		); err != nil {
			return fmt.Errorf("deployment failed: %w", err)
		}

		err := printConnectionInstructionsFromFile(commonFlags.DeploymentDir, os.Stdout)
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
