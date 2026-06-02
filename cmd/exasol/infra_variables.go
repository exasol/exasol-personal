// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/spf13/cobra"
)

var infraFlagToVarName = map[string]string{} // flag-name (hyphen) -> tofu var-name (underscore)

// infraPresetLabelAnnotationKey is a Cobra command annotation that stores the selected
// infrastructure preset label (either the embedded preset name or the preset path).
//
// It is used purely for rendering better help/usage output.
const infraPresetLabelAnnotationKey = "exasol.infrastructurePresetLabel"

const numberType = "number"

// numberFlag is a pflag.Value implementation that validates numbers without
// converting through float64 (to avoid precision loss).
//
// The raw string is kept as-is (trimmed) and later parsed by tofu.ParseOverrideStrings.
type numberFlag struct {
	raw string
}

func (n *numberFlag) String() string {
	return n.raw
}

func (n *numberFlag) Set(str string) error {
	str = strings.TrimSpace(str)
	if str == "" {
		// Allow empty string to behave like "not set"; Cobra will check flag.Changed.
		n.raw = ""
		return nil
	}

	num := new(big.Float)
	if _, ok := num.SetString(str); !ok {
		return fmt.Errorf("invalid number %q", str)
	}

	n.raw = str

	return nil
}

func (*numberFlag) Type() string {
	return numberType
}

func resolveInfrastructureVariables(
	infraPresetName string,
	infraPresetPath string,
) (map[string]*tofu.Variable, string, error) {
	var (
		info  *deploy.InfrastructureInfo
		err   error
		label string
	)

	preset := deploy.PresetRef{Name: infraPresetName, Path: infraPresetPath}
	info, err = deploy.GetInfrastructureInfoFromPreset(preset)
	if preset.IsPath() {
		label = infraPresetPath
	} else {
		label = infraPresetName
	}
	if err != nil {
		return nil, label, err
	}

	// If tofu is involved, read variables file from there
	if info.Tofu != nil {
		var variablesData []byte
		var variableFilepath string
		if preset.IsPath() {
			tofuCfg := tofu.NewTofuConfigFromPreset(infraPresetPath, *info.Tofu)
			variableFilepath = tofuCfg.VariablesFile()
			variablesData, err = os.ReadFile(variableFilepath)
		} else {
			variableFilepath = info.Tofu.VariablesFile
			variablesData, err = presets.ReadInfrastructureFile(infraPresetName, variableFilepath)
		}

		if err != nil {
			return nil, label, err
		}

		vars, err := tofu.ParseVarFile(variablesData, variableFilepath)
		if err != nil {
			return nil, label, err
		}

		return vars, label, nil
	}

	return map[string]*tofu.Variable{}, label, nil
}

// prepareInfrastructureVariableFlags registers infrastructure variable flags
// for commands that use them (currently: init, install).
//
// We scan the raw args for the selected infrastructure preset so that we can register
// only that preset's variables before Cobra parses.
func prepareInfrastructureVariableFlags(args []string) error {
	// Do not hard-fail before Cobra runs.
	// Reasons:
	//   - Users may ask for --help with an invalid preset value; help should still render.
	//   - Preset validity is validated later by the command (InitDeployment), which can
	//     provide a user-friendly error listing available presets.

	// We avoid scanning tokens ourselves; instead we use pflag to parse the already-known
	// flags (deployment-dir, log-level, etc.) and then scan the remaining positionals.
	preset, err := scanInfrastructurePresetSelection(args)
	if err != nil {
		return prepareConfigSetInfrastructureVariableFlags(args)
	}
	if preset != nil {
		vars, label, err := resolveInfrastructureVariables(preset.Name, preset.Path)
		if err == nil {
			if err := registerInfrastructureVariableFlags(
				[]*cobra.Command{initCmd, installCmd},
				vars,
				label,
			); err != nil {
				return err
			}
		}
	}

	return prepareConfigSetInfrastructureVariableFlags(args)
}

func prepareConfigSetInfrastructureVariableFlags(args []string) error {
	if !preregisteredCommandIs(args, configSetCmd) {
		return nil
	}

	if deployment, err := deploymentDirFromRawArgs(args); err == nil {
		vars, label, resolveErr := resolveInfrastructureVariablesFromDeployment(deployment)
		if resolveErr == nil {
			return registerInfrastructureVariableFlags(
				[]*cobra.Command{configSetCmd},
				vars,
				label,
			)
		}
	}

	return nil
}

func registerInfrastructureVariableFlags(
	cmds []*cobra.Command,
	vars map[string]*tofu.Variable,
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
		cmd.Annotations[infraPresetLabelAnnotationKey] = label

		for name, variable := range vars {
			if variable == nil {
				continue
			}

			flagName := strings.ReplaceAll(name, "_", "-")
			if cmd.Flags().Lookup(flagName) != nil || cmd.InheritedFlags().Lookup(flagName) != nil {
				return fmt.Errorf(
					"infrastructure variable flag name conflict: "+
						"--%s is already defined (preset: %s)",
					flagName,
					label,
				)
			}

			usage := variable.Description
			if strings.TrimSpace(usage) == "" {
				usage = "Infrastructure variable"
			}
			if !variable.Value.IsNull() && variable.Value.IsWhollyKnown() {
				usage += fmt.Sprintf(" (default: %s)", ctyToJSONString(variable.Value))
			}
			if variable.Required {
				usage += " (required)"
			}

			switch strings.TrimSpace(variable.Type) {
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
			infraFlagToVarName[flagName] = name
		}
	}

	return nil
}

func resolveInfrastructureVariablesFromDeployment(
	deployment config.DeploymentDir,
) (map[string]*tofu.Variable, string, error) {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return nil, "", err
	}
	label := manifest.Name
	if manifest.Tofu == nil {
		return map[string]*tofu.Variable{}, label, nil
	}

	tofuCfg := tofu.NewTofuConfigFromDeployment(deployment.Root(), *manifest.Tofu)
	variablesData, err := os.ReadFile(tofuCfg.VariablesFile())
	if err != nil {
		return nil, label, err
	}
	vars, err := tofu.ParseVarFile(variablesData, tofuCfg.VariablesFile())
	if err != nil {
		return nil, label, err
	}

	return vars, label, nil
}

func collectInfrastructureVariableOverrides(cmd *cobra.Command) map[string]string {
	overrides := map[string]string{}
	for flagName, varName := range infraFlagToVarName {
		flag := cmd.Flags().Lookup(flagName)
		if flag == nil {
			continue
		}
		if !flag.Changed {
			continue
		}
		// Use the flag's typed value rendering.
		// For bool flags this yields "true"/"false"; for numbers it yields the
		// validated raw number string; for strings it yields the string itself.
		overrides[varName] = flag.Value.String()
	}

	return overrides
}

func scanInfrastructurePresetSelection(args []string) (*deploy.PresetRef, error) {
	cmd, remainingArgs := preregisteredCommand(args)
	if cmd != initCmd && cmd != installCmd {
		return nil, errors.New("no command with infrastructure preset argument found")
	}

	positionals, err := preregisteredPositionals(remainingArgs)
	if err != nil {
		return nil, err
	}

	if len(positionals) == 0 {
		return nil, errors.New("infra preset not provided")
	}

	ref := presetRefFromArg(positionals[0])
	if strings.TrimSpace(ref.Name) == "" && strings.TrimSpace(ref.Path) == "" {
		return nil, errors.New("no valid preset value")
	}

	return &ref, nil
}
