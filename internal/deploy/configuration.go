// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

type DeploymentConfigValue struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Type    string `json:"type"`
	Value   any    `json:"value"`
	Default any    `json:"default"`

	VariableName string `json:"-"`
	RawValue     string `json:"-"`
	RawDefault   string `json:"-"`
}

type deploymentConfigRawValues struct {
	infraVars   map[string]string
	installVars map[string]string
}

const (
	ConfigScopeInfrastructure = "infrastructure"
	ConfigScopeInstallation   = "installation"
)

func GetDeploymentConfiguration(
	ctx context.Context,
	deployment config.DeploymentDir,
	optionNames []string,
) ([]DeploymentConfigValue, error) {
	var values []DeploymentConfigValue
	err := withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		var err error
		values, err = readDeploymentConfigurationValues(deployment)
		if err != nil {
			return err
		}
		values, err = filterDeploymentConfigurationValues(values, optionNames)

		return err
	})
	if err != nil {
		return nil, err
	}

	return values, nil
}

func SetDeploymentConfiguration(
	ctx context.Context,
	infraVars map[string]string,
	installVars map[string]string,
	deployment config.DeploymentDir,
) ([]DeploymentConfigValue, error) {
	if len(infraVars) == 0 && len(installVars) == 0 {
		return nil, errors.New("no configuration options were provided")
	}

	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}
			if err := WorkflowStatePermitsConfigure(exasolState); err != nil {
				return err
			}

			return writeDeploymentConfigurationPatch(
				ctx,
				deployment,
				exasolState,
				infraVars,
				installVars,
			)
		})
	if err != nil {
		return nil, err
	}

	return GetDeploymentConfiguration(ctx, deployment, nil)
}

//nolint:revive // resetAll mirrors the command-level --all flag.
func ResetDeploymentConfiguration(
	ctx context.Context,
	deployment config.DeploymentDir,
	optionNames []string,
	resetAll bool,
) ([]DeploymentConfigValue, error) {
	if !resetAll && len(optionNames) == 0 {
		return nil, errors.New("provide option names to reset, or pass --all")
	}
	if resetAll && len(optionNames) > 0 {
		return nil, errors.New("pass either --all or option names, not both")
	}

	err := withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				return err
			}
			if err := WorkflowStatePermitsConfigure(exasolState); err != nil {
				return err
			}

			values, err := readDeploymentConfigurationValues(deployment)
			if err != nil {
				return err
			}
			if !resetAll {
				values, err = resetSelectedConfigurationValues(values, optionNames)
				if err != nil {
					return err
				}
			} else {
				for idx := range values {
					values[idx].RawValue = values[idx].RawDefault
					values[idx].Value = values[idx].Default
				}
			}

			rawValues := splitConfigurationRawValues(values)

			return writeDeploymentConfiguration(
				ctx,
				deployment,
				exasolState,
				rawValues.infraVars,
				rawValues.installVars,
			)
		})
	if err != nil {
		return nil, err
	}

	return GetDeploymentConfiguration(ctx, deployment, nil)
}

func readDeploymentConfigurationValues(
	deployment config.DeploymentDir,
) ([]DeploymentConfigValue, error) {
	infraManifest, installManifest, err := readExtractedManifests(deployment)
	if err != nil {
		return nil, err
	}
	backend, err := resolveBackendForManifest(infraManifest)
	if err != nil {
		return nil, err
	}
	infraValues, err := backend.ReadConfiguration(deployment, infraManifest)
	if err != nil {
		return nil, err
	}
	installValues, err := readInstallationConfigurationValues(deployment, installManifest)
	if err != nil {
		return nil, err
	}

	values := append([]DeploymentConfigValue{}, infraValues...)
	values = append(values, installValues...)
	sortConfigurationValues(values)

	return values, nil
}

func filterDeploymentConfigurationValues(
	values []DeploymentConfigValue,
	optionNames []string,
) ([]DeploymentConfigValue, error) {
	if len(optionNames) == 0 {
		return values, nil
	}

	byName := map[string]DeploymentConfigValue{}
	for _, value := range values {
		byName[normalizeConfigOptionName(value.Name)] = value
	}

	filtered := make([]DeploymentConfigValue, 0, len(optionNames))
	for _, optionName := range optionNames {
		normalized := normalizeConfigOptionName(optionName)
		value, ok := byName[normalized]
		if !ok {
			return nil, fmt.Errorf("unknown configuration option %q", optionName)
		}
		filtered = append(filtered, value)
	}

	return filtered, nil
}

func resetSelectedConfigurationValues(
	values []DeploymentConfigValue,
	optionNames []string,
) ([]DeploymentConfigValue, error) {
	selected := map[string]struct{}{}
	for _, optionName := range optionNames {
		selected[normalizeConfigOptionName(optionName)] = struct{}{}
	}

	for idx := range values {
		normalized := normalizeConfigOptionName(values[idx].Name)
		if _, ok := selected[normalized]; !ok {
			continue
		}
		values[idx].RawValue = values[idx].RawDefault
		values[idx].Value = values[idx].Default
		delete(selected, normalized)
	}

	if len(selected) > 0 {
		missing := make([]string, 0, len(selected))
		for optionName := range selected {
			missing = append(missing, optionName)
		}
		sort.Strings(missing)

		return nil, fmt.Errorf("unknown configuration option %q", missing[0])
	}

	return values, nil
}

func splitConfigurationRawValues(values []DeploymentConfigValue) deploymentConfigRawValues {
	rawValues := deploymentConfigRawValues{
		infraVars:   map[string]string{},
		installVars: map[string]string{},
	}
	for _, value := range values {
		switch value.Scope {
		case ConfigScopeInfrastructure:
			rawValues.infraVars[value.VariableName] = value.RawValue
		case ConfigScopeInstallation:
			rawValues.installVars[value.VariableName] = value.RawValue
		default:
			continue
		}
	}

	return rawValues
}

func rawInfrastructureConfigValues(values map[string]string) []DeploymentConfigValue {
	result := make([]DeploymentConfigValue, 0, len(values))
	for name, rawValue := range values {
		result = append(result, DeploymentConfigValue{
			Name:         configOptionDisplayName(name),
			Scope:        ConfigScopeInfrastructure,
			VariableName: name,
			RawValue:     rawValue,
		})
	}
	sortConfigurationValues(result)

	return result
}

func configValuesRawMap(values []DeploymentConfigValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		name := value.VariableName
		if name == "" {
			name = strings.TrimSpace(value.Name)
		}
		if name == "" {
			continue
		}
		result[name] = value.RawValue
	}

	return result
}

func writeDeploymentConfigurationPatch(
	ctx context.Context,
	deployment config.DeploymentDir,
	exasolState *config.ExasolPersonalState,
	infraVars map[string]string,
	installVars map[string]string,
) error {
	values, err := readDeploymentConfigurationValues(deployment)
	if err != nil {
		return err
	}
	rawValues := splitConfigurationRawValues(values)
	for key, value := range infraVars {
		rawValues.infraVars[key] = value
	}
	for key, value := range installVars {
		rawValues.installVars[key] = value
	}

	return writeDeploymentConfiguration(
		ctx,
		deployment,
		exasolState,
		rawValues.infraVars,
		rawValues.installVars,
	)
}

func readInstallationConfigurationValues(
	deployment config.DeploymentDir,
	manifest *presets.InstallManifest,
) ([]DeploymentConfigValue, error) {
	if manifest == nil || manifest.Variables == nil || len(manifest.Variables.Vars) == 0 {
		return []DeploymentConfigValue{}, nil
	}

	currentValues, err := readCurrentInstallationVariables(deployment, manifest)
	if err != nil {
		return nil, err
	}

	values := make([]DeploymentConfigValue, 0, len(manifest.Variables.Vars))
	for name, def := range manifest.Variables.Vars {
		name = strings.TrimSpace(name)
		if name == "" || def == nil || isReservedInstallationVariableName(name) {
			continue
		}
		defaultValue, err := def.DefaultScalar()
		if err != nil {
			return nil, fmt.Errorf("invalid default for installation variable %q: %w", name, err)
		}
		effectiveType, err := def.EffectiveType()
		if err != nil {
			return nil, fmt.Errorf("invalid definition of installation variable %q: %w", name, err)
		}
		value := defaultValue
		if current, ok := currentValues[name]; ok {
			value = current
		}
		rawValue, err := scalarToRawString(value)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid current value for installation variable %q: %w",
				name,
				err,
			)
		}
		rawDefault, err := scalarToRawString(defaultValue)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid default value for installation variable %q: %w",
				name,
				err,
			)
		}
		values = append(values, DeploymentConfigValue{
			Name:         configOptionDisplayName(name),
			Scope:        ConfigScopeInstallation,
			Type:         effectiveType,
			Value:        value,
			Default:      defaultValue,
			VariableName: name,
			RawValue:     rawValue,
			RawDefault:   rawDefault,
		})
	}

	return values, nil
}

func readCurrentInstallationVariables(
	deployment config.DeploymentDir,
	manifest *presets.InstallManifest,
) (map[string]any, error) {
	result := map[string]any{}
	if manifest == nil || manifest.Variables == nil {
		return result, nil
	}
	outputRel := strings.TrimSpace(manifest.Variables.OutputFile)
	if outputRel == "" {
		return result, nil
	}
	outPath := filepath.Join(deployment.InstallationDir(), filepath.Clean(outputRel))
	data, err := os.ReadFile(outPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to read installation variables: %w", err)
	}

	return result, nil
}

func scalarToRawString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		return typed.String(), nil
	default:
		return "", fmt.Errorf("unsupported scalar type %T", value)
	}
}

func isReservedInfrastructureVariableName(name string) bool {
	switch strings.TrimSpace(name) {
	case "deployment_id",
		"cluster_identity",
		"deployment_created_at",
		"infrastructure_artifact_dir",
		"installation_preset_dir":
		return true
	default:
		return false
	}
}

func normalizeConfigOptionName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", "-")
}

func configOptionDisplayName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", "-")
}

func sortConfigurationValues(values []DeploymentConfigValue) {
	sort.Slice(values, func(left, right int) bool {
		if values[left].Scope != values[right].Scope {
			return values[left].Scope < values[right].Scope
		}

		return values[left].Name < values[right].Name
	})
}
