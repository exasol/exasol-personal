// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

// flag-name (hyphen) -> install var-name (underscore).
var installFlagToVarName = map[string]string{}

// installPresetLabelAnnotationKey is a Cobra command annotation that stores the selected
// installation preset label (either the embedded preset name or the preset path).
//
// It is used purely for rendering better help/usage output.
const installPresetLabelAnnotationKey = "exasol.installationPresetLabel"

func resolveInstallationVariables(
	installPresetName string,
	installPresetPath string,
) (map[string]*presets.VariableDef, string, error) {
	var (
		manifest *presets.InstallManifest
		err      error
		label    string
	)

	preset := deploy.PresetRef{Name: installPresetName, Path: installPresetPath}
	if preset.IsPath() {
		label = installPresetPath
		manifest, err = presets.ReadInstallManifestFromDir(installPresetPath)
	} else {
		label = installPresetName
		manifest, err = presets.ReadInstallManifest(installPresetName)
	}
	if err != nil {
		return nil, label, err
	}

	if manifest == nil || manifest.Variables == nil || len(manifest.Variables.Vars) == 0 {
		return map[string]*presets.VariableDef{}, label, nil
	}

	return manifest.Variables.Vars, label, nil
}

// prepareInstallationVariableFlags registers installation variable flags
// for commands that use them (currently: init, install).
//
// We scan the raw args for the selected installation preset so that we can register
// only that preset's variables before Cobra parses.
func prepareInstallationVariableFlags(args []string) error {
	// Be tolerant and avoid hard failures before Cobra runs.
	preset, err := scanInstallationPresetSelection(args)
	if err != nil {
		return prepareConfigSetInstallationVariableFlags(args)
	}
	if preset != nil {
		vars, label, err := resolveInstallationVariables(preset.Name, preset.Path)
		if err == nil {
			if err := registerInstallationVariableFlags(
				[]*cobra.Command{initCmd, installCmd},
				vars,
				label,
			); err != nil {
				return err
			}
		}
	}

	return prepareConfigSetInstallationVariableFlags(args)
}

func prepareConfigSetInstallationVariableFlags(args []string) error {
	if !preregisteredCommandIs(args, configSetCmd) {
		return nil
	}

	if deployment, err := deploymentDirFromRawArgs(args); err == nil {
		vars, label, resolveErr := resolveInstallationVariablesFromDeployment(deployment)
		if resolveErr == nil {
			return registerInstallationVariableFlags(
				[]*cobra.Command{configSetCmd},
				vars,
				label,
			)
		}
	}

	return nil
}

func registerInstallationVariableFlags(
	cmds []*cobra.Command,
	vars map[string]*presets.VariableDef,
	label string,
) error {
	if len(vars) == 0 {
		return nil
	}

	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[installPresetLabelAnnotationKey] = label

		for name, def := range vars {
			if def == nil {
				continue
			}

			flagName := strings.ReplaceAll(name, "_", "-")
			if cmd.Flags().Lookup(flagName) != nil || cmd.InheritedFlags().Lookup(flagName) != nil {
				return fmt.Errorf(
					"installation variable flag name conflict: "+
						"--%s is already defined (preset: %s)",
					flagName,
					label,
				)
			}

			usage := strings.TrimSpace(def.Description)
			if usage == "" {
				usage = "Installation variable"
			}
			if def.Default != nil {
				usage += fmt.Sprintf(" (default: %v)", def.Default)
			}

			effectiveType, err := def.EffectiveType()
			if err != nil {
				return fmt.Errorf("invalid definition of installation variable %q: %w", name, err)
			}
			switch effectiveType {
			case "bool":
				cmd.Flags().Bool(flagName, false, usage)
			case "number":
				cmd.Flags().Var(&numberFlag{}, flagName, usage)
			default:
				cmd.Flags().String(flagName, "", usage)
			}
			if f := cmd.Flags().Lookup(flagName); f != nil {
				f.DefValue = ""
			}
			installFlagToVarName[flagName] = name
		}
	}

	return nil
}

func resolveInstallationVariablesFromDeployment(
	deployment config.DeploymentDir,
) (map[string]*presets.VariableDef, string, error) {
	manifest, err := config.ReadInstallManifest(deployment)
	if err != nil {
		return nil, "", err
	}
	if manifest == nil || manifest.Variables == nil || len(manifest.Variables.Vars) == 0 {
		return map[string]*presets.VariableDef{}, manifest.Name, nil
	}

	return manifest.Variables.Vars, manifest.Name, nil
}

func collectInstallationVariableOverrides(cmd *cobra.Command) map[string]string {
	overrides := map[string]string{}
	for flagName, varName := range installFlagToVarName {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			continue
		}
		if !flag.Changed {
			continue
		}
		overrides[varName] = flag.Value.String()
	}

	return overrides
}

func scanInstallationPresetSelection(args []string) (*deploy.PresetRef, error) {
	if len(args) > 0 && args[0] == helpCommandName {
		args = args[1:]
	}
	cmd, remainingArgs := preregisteredCommand(args)
	if cmd != initCmd && cmd != installCmd {
		return nil, errors.New("no command with installation preset argument found")
	}

	positionals, err := preregisteredPositionals(remainingArgs)
	if err != nil {
		return nil, err
	}

	if len(positionals) == 0 {
		return nil, errors.New("infra preset not provided")
	}

	if len(positionals) > 1 {
		ref, err := resolvePresetRef(
			context.Background(), positionals[1], presets.PresetTypeInstallation,
		)
		if err != nil {
			return nil, err
		}

		return &ref, nil
	}

	infraRef, err := resolvePresetRef(
		context.Background(), positionals[0], presets.PresetTypeInfrastructure,
	)
	if err != nil {
		return nil, err
	}
	ref, err := deploy.ResolveDefaultInstallationPreset(infraRef)
	if err != nil {
		return nil, err
	}

	return &ref, nil
}
