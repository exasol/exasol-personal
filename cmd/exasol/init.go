// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

const (
	minPresetArgs = 1
	maxPresetArgs = 2
)

var initCmdShortDesc = `Initialize a new deployment directory`

const (
	deploymentDirectoryResolutionHelp = `
	If --deployment-dir is not provided and the current directory is not a deployment directory,
	uses ~/.exasol/personal/deployments/default.
`
	presetSelectionHelp = `
	Preset arguments:
	  - The first argument selects the infrastructure preset (required).
	  - The optional second argument selects the installation preset.

	Each argument can be either an embedded preset name (e.g. "aws") or a preset directory path.
	To force path selection, pass a path-like value such as "./my-preset" or "/abs/path/to/preset".

	Tip: use "exasol presets" to discover and export presets.
	`
)

var initCmdLongDesc = initCmdShortDesc + `

	Extracts the specified infrastructure and installation presets into the deployment directory.
	For an already initialized deployment with the same presets, supplied configuration
	flags update only the selected parameters. Omitted parameters keep their values.
	To switch presets, run exasol destroy --remove first, or exasol remove if the
	deployment resources are already gone.` +
	deploymentDirectoryResolutionHelp + presetSelectionHelp

var initCmd = &cobra.Command{
	Use:   "init <infra preset name-or-path> [install preset name-or-path]",
	Short: initCmdShortDesc,
	Long:  initCmdLongDesc,
	Example: "  exasol init " + presets.DefaultInfrastructure + "\n" +
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

	if matrix := embeddedPresetCompatibilityMatrix(); matrix != "" {
		initCmd.Long += "\n\n\t" + strings.ReplaceAll(matrix, "\n", "\n\t")
	}

	initCmd.RunE = func(cmd *cobra.Command, args []string) error {
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
			return runInitForInitializedDeployment(
				cmd,
				deployment,
				infraPreset,
				installPreset,
				infraVars,
				installVars,
			)
		}

		return runInitForFreshDeployment(
			cmd,
			deployment,
			infraPreset,
			installPreset,
			infraVars,
			installVars,
		)
	}

	registerInitFlags(initCmd, commonFlags)
	registerDeploymentDirFlag(initCmd, commonFlags)
	rootCmd.AddCommand(initCmd)
}

func runInitForInitializedDeployment(
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
	if err := setupDeploymentLogSession(cmd, deployment); err != nil {
		return err
	}
	values, changed, err := applyConfigurationPatchIfProvided(cmd, infraVars, installVars)
	if err != nil {
		return err
	}
	if changed {
		addTerminalNotice(formatConfigurationChangedNotice(values))

		return nil
	}
	addTerminalNotice(
		"deployment directory is already initialized with the requested presets; " +
			"configuration was not changed. Run `exasol config set` to " +
			"update configuration.",
	)

	return nil
}

func runInitForFreshDeployment(
	cmd *cobra.Command,
	deployment config.DeploymentDir,
	infraPreset deploy.PresetRef,
	installPreset deploy.PresetRef,
	infraVars map[string]string,
	installVars map[string]string,
) error {
	err := deploy.InitDeployment(
		cmd.Context(),
		infraPreset,
		installPreset,
		infraVars,
		installVars,
		deployment,
		!commonFlags.NoLauncherVersionCheck,
		CurrentLauncherVersion,
	)
	if err != nil {
		return err
	}
	if err := setupDeploymentLogSession(cmd, deployment); err != nil {
		return err
	}
	addTerminalNotice(deploy.EulaNoticeText)
	addTerminalNotice(deploy.LocalInitMemoryNotice(deployment))

	return nil
}

func ensureRequestedPresetsMatchInitializedDeployment(
	deployment config.DeploymentDir,
	infraPreset deploy.PresetRef,
	installPreset deploy.PresetRef,
) error {
	if err := deploy.ValidatePresetSelection(infraPreset, installPreset); err != nil {
		return err
	}

	return deploy.EnsureDeploymentPresetIdentityMatches(
		deployment,
		infraPreset,
		installPreset,
	)
}
