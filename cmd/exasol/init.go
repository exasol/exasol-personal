// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

const (
	minPresetArgs = 1
	maxPresetArgs = 2
)

var initCmdShortDesc = `Initialize a new deployment directory`

var initCmdLongDesc = initCmdShortDesc + `

	Extracts the specified infrastructure and installation presets into the deployment directory.

	Preset arguments:
	  - The first argument selects the infrastructure preset (required).
	  - The optional second argument selects the installation preset.

	Each argument can be either an embedded preset name (e.g. "aws") or a preset directory path.
	To force path selection, pass a path-like value such as "./my-preset" or "/abs/path/to/preset".

	Tip: use "exasol presets" to discover and export presets.
	`

var initCmd = &cobra.Command{
	Use:   "init <infra preset name-or-path> [install preset name-or-path]",
	Short: initCmdShortDesc,
	Long:  initCmdLongDesc,
	Example: "  exasol init " +
		presets.DefaultInfrastructure + "\n" +
		"  exasol init " + presets.DefaultInfrastructure + " " +
		presets.DefaultInstallation + "\n" +
		"  exasol init ./my-infra-preset ./my-install-preset",
	Args:    cobra.RangeArgs(minPresetArgs, maxPresetArgs),
	GroupID: rootCmdGroupEssential,
}

// nolint: gochecknoinits
func init() {
	// Init creates the deployment directory state; if a deployment directory already
	// exists, the compatibility check protects against operating on an incompatible
	// deployment.
	requireMinorVersionCompatibility(initCmd, minSupportedDeploymentVersionBaseline)
	requireDeploymentFileLogging(initCmd)

	// Augment long help with embedded preset names so users know what they can pass.
	initCmd.Long = strings.TrimRight(initCmdLongDesc, "\n") +
		"\n\t" + presetNamesForHelp(presets.PresetTypeInfrastructure,
		presets.ListEmbeddedInfrastructuresPresets()) +
		"\n\t" + presetNamesForHelp(presets.PresetTypeInstallation,
		presets.ListEmbeddedInstallationsPresets())

	initCmd.RunE = func(cmd *cobra.Command, args []string) error {
		infraVars := collectInfrastructureVariableOverrides(cmd)
		installVars := collectInstallationVariableOverrides(cmd)
		infraPreset := presetRefFromArg(args[0])
		installPreset := defaultedPresetRefFromOptionalArg(
			args, 1, defaultInstallationPresetRefForInfra(infraPreset))

		safePrint(deploy.EulaNoticeText)

		err := deploy.InitDeployment(
			cmd.Context(),
			infraPreset,
			installPreset,
			infraVars,
			installVars,
			commonFlags.DeploymentDir,
			!commonFlags.NoLauncherVersionCheck,
			CurrentLauncherVersion,
		)
		if err != nil {
			return err
		}

		return setupDeploymentLogSession(cmd, commonFlags.DeploymentDir)
	}

	registerInitFlags(initCmd, commonFlags)
	registerDeploymentDirFlag(initCmd, commonFlags)
	rootCmd.AddCommand(initCmd)
}
