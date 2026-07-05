// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/spf13/cobra"
)

var infraFlagToVarName = map[string]string{} // flag-name (hyphen) -> infra var-name (underscore)

// infraPresetLabelAnnotationKey is a Cobra command annotation that stores the selected
// infrastructure preset label (either the embedded preset name or the preset path).
//
// It is used purely for rendering better help/usage output.
const infraPresetLabelAnnotationKey = "exasol.infrastructurePresetLabel"

const numberType = "number"

// numberFlag is a pflag.Value implementation that validates numbers without
// converting through float64 (to avoid precision loss).
//
// The raw string is kept as-is (trimmed) and later parsed by the infrastructure backend.
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
) (deploy.ConfigVariableResolution, error) {
	preset := deploy.PresetRef{Name: infraPresetName, Path: infraPresetPath}

	return deploy.ResolveInfrastructureConfigVariables(preset)
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
		resolution, err := resolveInfrastructureVariables(preset.Name, preset.Path)
		if err == nil {
			if err := registerInfrastructureVariableFlags(
				[]*cobra.Command{initCmd, installCmd},
				resolution.Variables,
				resolution.PresetLabel,
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

	deployment, err := deploymentDirFromRawArgs(args)
	if err != nil {
		if rawArgsRequestHelp(args) {
			return nil
		}

		return fmt.Errorf("cannot determine deployment directory for `config set`: %w", err)
	}

	resolution, resolveErr := resolveInfrastructureVariablesFromDeployment(deployment)
	if resolveErr != nil {
		// Keep `config set --help` working even when options cannot be loaded:
		// render the base help instead of failing.
		if rawArgsRequestHelp(args) {
			return nil
		}

		// Fail with a clear message instead of silently registering no flags. Otherwise the
		// supplied options fail Cobra's flag parsing later as a misleading "unknown flag"
		// error before the initialized-deployment pre-run gate can explain the problem.
		return fmt.Errorf(
			"%w: %q\n"+
				"Run `exasol init` or `exasol install` there, "+
				"or pass --deployment-dir pointing to an existing deployment directory",
			deploy.ErrNotExasolPersonalDeploymentDirectory,
			deployment.Root(),
		)
	}

	return registerInfrastructureVariableFlags(
		[]*cobra.Command{configSetCmd},
		resolution.Variables,
		resolution.PresetLabel,
	)
}

// rawArgsRequestHelp reports whether the raw command-line args request help, in which
// case flag pre-registration must stay non-fatal so help can render.
func rawArgsRequestHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}

	return len(args) > 0 && args[0] == helpCommandName
}

func registerInfrastructureVariableFlags(
	cmds []*cobra.Command,
	vars map[string]deploy.ConfigVariableDefinition,
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
			if strings.TrimSpace(variable.Name) != "" {
				name = variable.Name
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

			usage := strings.TrimSpace(variable.Description)
			if strings.TrimSpace(usage) == "" {
				usage = "Infrastructure variable"
			}
			if strings.TrimSpace(variable.DefaultDisplay) != "" {
				usage += fmt.Sprintf(" (default: %s)", variable.DefaultDisplay)
			}
			if variable.Required {
				usage += " (required)"
			}

			switch variable.Type {
			case deploy.ConfigVariableTypeBool:
				cmd.Flags().Bool(flagName, false, usage)
			case deploy.ConfigVariableTypeNumber:
				cmd.Flags().Var(&numberFlag{}, flagName, usage)
			case deploy.ConfigVariableTypeString:
				cmd.Flags().String(flagName, "", usage)
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
) (deploy.ConfigVariableResolution, error) {
	return deploy.ResolveInfrastructureConfigVariablesFromDeployment(deployment)
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
	// Handle "exasol help init local" form: strip the leading "help" token so the
	// preset scanner finds the actual command. Cobra registers its help subcommand
	// lazily inside ExecuteC, so rootCmd.Find won't resolve it at this point.
	if len(args) > 0 && args[0] == helpCommandName {
		args = args[1:]
	}
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
