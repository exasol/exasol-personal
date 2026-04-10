// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		// nolint: nilerr
		return nil
	}
	if preset == nil {
		return nil
	}

	vars, label, err := resolveInstallationVariables(preset.Name, preset.Path)
	if err != nil {
		_ = label
		// nolint: nilerr
		return nil
	}
	if len(vars) == 0 {
		return nil
	}

	cmds := []*cobra.Command{initCmd, installCmd}
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
	flagset := pflag.NewFlagSet("install-preset-scan", pflag.ContinueOnError)
	flagset.SetOutput(io.Discard)
	flagset.SetInterspersed(true)
	flagset.ParseErrorsAllowlist.UnknownFlags = true
	flagset.BoolP("help", "h", false, "")

	if err := flagset.Parse(args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return nil, fmt.Errorf("cannot parse pre-init flags: %w", err)
	}

	positionals := flagset.Args()
	cmdIndex := -1
	for i, tok := range positionals {
		if tok == "init" || tok == "install" {
			cmdIndex = i
			break
		}
	}
	if cmdIndex < 0 {
		return nil, errors.New("no commands with preset arguments found")
	}
	if cmdIndex+1 >= len(positionals) {
		return nil, errors.New("infra preset not provided")
	}

	infraPreset := presetRefFromArg(positionals[cmdIndex+1])

	// Optional installation preset argument; default when omitted.
	defaultInstall := defaultInstallationPresetRefForInfra(infraPreset)
	installArg := defaultInstall.Name
	if defaultInstall.IsPath() {
		installArg = defaultInstall.Path
	}
	if cmdIndex+2 < len(positionals) {
		installArg = positionals[cmdIndex+2]
	}

	ref := presetRefFromArg(installArg)
	if strings.TrimSpace(ref.Name) == "" && strings.TrimSpace(ref.Path) == "" {
		return nil, errors.New("no valid preset value")
	}

	return &ref, nil
}
