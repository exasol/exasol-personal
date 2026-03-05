// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
)

const (
	installVariablesPermissions       = 0o600
	installVariablesFolderPermissions = 0o750
)

func writeInstallationVariablesFile(
	installDir string,
	spec *presets.Variables,
	clusterIdentity string,
	deploymentId string,
	overrides map[string]string,
) error {
	if spec == nil {
		return nil
	}
	outputRel := strings.TrimSpace(spec.OutputFile)
	if outputRel == "" {
		return nil
	}

	// Resolve and validate output path (must stay within installDir).
	outPath := filepath.Join(installDir, filepath.Clean(outputRel))
	installDirClean := filepath.Clean(installDir)
	if !strings.HasPrefix(outPath, installDirClean+string(os.PathSeparator)) &&
		outPath != installDirClean {
		return fmt.Errorf(
			"installation variables outputFile escapes installation directory: %q",
			outputRel,
		)
	}

	resolved := map[string]any{}
	// Always include launcher-governed identity values.
	resolved["deployment_id"] = deploymentId
	resolved["cluster_identity"] = clusterIdentity

	// Apply defaults from manifest.
	for name, def := range spec.Vars {
		name = strings.TrimSpace(name)
		if name == "" || def == nil {
			continue
		}
		if isReservedInstallationVariableName(name) {
			// Prevent accidental override of launcher-governed keys.
			continue
		}
		value, err := def.DefaultScalar()
		if err != nil {
			return fmt.Errorf("invalid default for installation variable %q: %w", name, err)
		}
		resolved[name] = value
	}

	// Apply CLI overrides.
	for name, raw := range overrides {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if isReservedInstallationVariableName(name) {
			continue
		}
		def := spec.Vars[name]
		if def == nil {
			// Ignore overrides for unknown variables. This keeps init/install tolerant
			// even when presets evolve.
			continue
		}
		effectiveType, err := def.EffectiveType()
		if err != nil {
			return fmt.Errorf("invalid definition of installation variable %q: %w", name, err)
		}
		val, err := parseInstallVarValue(effectiveType, raw)
		if err != nil {
			return fmt.Errorf("invalid value for installation variable %q: %w", name, err)
		}
		resolved[name] = val
	}

	if err := os.MkdirAll(filepath.Dir(outPath), installVariablesFolderPermissions); err != nil {
		return fmt.Errorf("failed to create installation variables output directory: %w", err)
	}
	data, err := json.MarshalIndent(resolved, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to encode installation variables JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outPath, data, installVariablesPermissions); err != nil {
		return fmt.Errorf("failed to write installation variables file: %w", err)
	}

	return nil
}

func isReservedInstallationVariableName(name string) bool {
	switch strings.TrimSpace(name) {
	case "deployment_id", "cluster_identity":
		return true
	default:
		return false
	}
}

func parseInstallVarValue(varType string, raw string) (any, error) {
	varType = strings.TrimSpace(varType)
	switch varType {
	case "bool":
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}

		return b, nil
	case "number":
		// Keep as float64 for JSON; jq and scripts can handle it.
		n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return nil, err
		}

		return n, nil
	case "", "string":
		return raw, nil
	default:
		return nil, fmt.Errorf("unsupported type %q", varType)
	}
}
