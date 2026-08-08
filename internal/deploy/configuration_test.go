// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

const testRegionDefault = "us-east-1"

func TestDeploymentConfigValueDisplayNameHyphenatesUnderscores(t *testing.T) {
	t.Parallel()

	value := DeploymentConfigValue{Name: "instance_count"}

	if got := value.DisplayName(); got != "instance-count" {
		t.Fatalf("expected 'instance-count', got %q", got)
	}
}

func TestNormalizeConfigOptionName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"instance-count": "instance_count",
		"instance_count": "instance_count",
		"  region  ":     "region",
	}
	for input, want := range cases {
		if got := normalizeConfigOptionName(input); got != want {
			t.Errorf("normalizeConfigOptionName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFilterDeploymentConfiguration_SelectsMatchingOptionsRegardlessOfHyphenation(t *testing.T) {
	t.Parallel()

	configuration := newDeploymentConfigurationFromRaw(
		map[string]string{"instance_count": "3", "region": "eu-central-1"},
		map[string]string{"admin_password": "secret"},
	)

	filtered, err := filterDeploymentConfiguration(configuration, []string{"instance-count"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(filtered.Infrastructure.Options) != 1 ||
		filtered.Infrastructure.Options[0].Name != "instance_count" {
		t.Fatalf(
			"expected only instance_count to be selected, got %+v",
			filtered.Infrastructure.Options,
		)
	}
	if len(filtered.Installation.Options) != 0 {
		t.Fatalf(
			"expected no installation options selected, got %+v",
			filtered.Installation.Options,
		)
	}
}

func TestFilterDeploymentConfiguration_NoNamesReturnsEverything(t *testing.T) {
	t.Parallel()

	configuration := newDeploymentConfigurationFromRaw(
		map[string]string{"region": "eu-central-1"},
		nil,
	)

	filtered, err := filterDeploymentConfiguration(configuration, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(filtered.Infrastructure.Options) != 1 {
		t.Fatalf("expected the full configuration to be returned unfiltered, got %+v", filtered)
	}
}

func TestFilterDeploymentConfiguration_UnknownOptionReturnsError(t *testing.T) {
	t.Parallel()

	configuration := newDeploymentConfigurationFromRaw(
		map[string]string{"region": "eu-central-1"},
		nil,
	)

	_, err := filterDeploymentConfiguration(configuration, []string{"does-not-exist"})
	if err == nil {
		t.Fatal("expected an error for an unknown configuration option")
	}
}

func TestResetSelectedDeploymentConfiguration_ResetsOnlySelectedOptions(t *testing.T) {
	t.Parallel()

	configuration := DeploymentConfiguration{
		Infrastructure: DeploymentConfigurationSection{
			Options: []DeploymentConfigValue{
				{
					Name:       "region",
					RawValue:   "eu-west-1",
					RawDefault: testRegionDefault,
					Default:    testRegionDefault,
				},
				{Name: "instance_count", RawValue: "5", RawDefault: "2", Default: 2},
			},
		},
	}

	reset, err := resetSelectedDeploymentConfiguration(configuration, []string{"region"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if reset.Infrastructure.Options[0].RawValue != testRegionDefault {
		t.Fatalf(
			"expected region to be reset to its default, got %+v",
			reset.Infrastructure.Options[0],
		)
	}
	if reset.Infrastructure.Options[1].RawValue != "5" {
		t.Fatalf(
			"expected instance_count to be untouched, got %+v",
			reset.Infrastructure.Options[1],
		)
	}
}

func TestResetSelectedDeploymentConfiguration_UnknownOptionReturnsError(t *testing.T) {
	t.Parallel()

	configuration := DeploymentConfiguration{
		Infrastructure: DeploymentConfigurationSection{
			Options: []DeploymentConfigValue{{Name: "region"}},
		},
	}

	_, err := resetSelectedDeploymentConfiguration(configuration, []string{"does-not-exist"})
	if err == nil {
		t.Fatal("expected an error for an unknown configuration option")
	}
}

func TestResetAllConfigurationValues_ResetsEveryOption(t *testing.T) {
	t.Parallel()

	values := []DeploymentConfigValue{
		{
			Name:       "region",
			RawValue:   "eu-west-1",
			RawDefault: testRegionDefault,
			Default:    testRegionDefault,
		},
		{Name: "instance_count", RawValue: "5", RawDefault: "2", Default: 2},
	}

	resetAllConfigurationValues(values)

	if values[0].RawValue != testRegionDefault || values[1].RawValue != "2" {
		t.Fatalf("expected all values to reset to their defaults, got %+v", values)
	}
}

func TestConfigValuesRawMap_SkipsBlankNames(t *testing.T) {
	t.Parallel()

	values := []DeploymentConfigValue{
		{Name: "region", RawValue: "eu-west-1"},
		{Name: "  ", RawValue: "ignored"},
	}

	result := configValuesRawMap(values)

	if len(result) != 1 || result["region"] != "eu-west-1" {
		t.Fatalf("expected only 'region' to be included, got %+v", result)
	}
}

func TestScalarToRawString(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value   any
		want    string
		wantErr bool
	}{
		"string":         {value: "hello", want: "hello"},
		"bool":           {value: true, want: "true"},
		"int":            {value: 42, want: "42"},
		"int64":          {value: int64(42), want: "42"},
		"float64":        {value: 3.5, want: "3.5"},
		"json.Number":    {value: json.Number("7"), want: "7"},
		"unsupported":    {value: []string{"x"}, wantErr: true},
		"struct is nope": {value: struct{}{}, wantErr: true},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := scalarToRawString(testCase.value)

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func newInitializedRealDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	return deployment
}

func findClusterSizeConfigValue(
	configuration DeploymentConfiguration,
) (DeploymentConfigValue, bool) {
	for _, value := range configuration.Infrastructure.Options {
		if value.Name == "cluster_size" {
			return value, true
		}
	}

	return DeploymentConfigValue{}, false
}

func TestResetDeploymentConfiguration_RejectsMissingSelector(t *testing.T) {
	t.Parallel()

	deployment := newInitializedRealDeployment(t)

	_, err := ResetDeploymentConfiguration(context.Background(), deployment, nil, false)
	if err == nil {
		t.Fatal("expected an error when neither option names nor --all is given")
	}
}

func TestResetDeploymentConfiguration_RejectsBothAllAndOptionNames(t *testing.T) {
	t.Parallel()

	deployment := newInitializedRealDeployment(t)

	_, err := ResetDeploymentConfiguration(
		context.Background(), deployment, []string{"cluster_size"}, true,
	)
	if err == nil {
		t.Fatal("expected an error when both --all and option names are given")
	}
}

func TestResetDeploymentConfiguration_ResetSelectedOptionRestoresItsDefault(t *testing.T) {
	t.Parallel()

	deployment := newInitializedRealDeployment(t)

	before, err := GetDeploymentConfiguration(context.Background(), deployment, nil)
	if err != nil {
		t.Fatalf("failed to read initial configuration: %v", err)
	}
	clusterSize, found := findClusterSizeConfigValue(before)
	if !found {
		t.Fatal("expected the aws preset to declare a cluster_size variable")
	}
	defaultRawValue := clusterSize.RawDefault

	overrideValue := "5"
	if defaultRawValue == overrideValue {
		overrideValue = "6"
	}
	if _, err := SetDeploymentConfiguration(
		context.Background(),
		map[string]string{"cluster_size": overrideValue},
		nil,
		deployment,
	); err != nil {
		t.Fatalf("failed to override cluster_size: %v", err)
	}

	after, err := ResetDeploymentConfiguration(
		context.Background(), deployment, []string{"cluster_size"}, false,
	)
	if err != nil {
		t.Fatalf("failed to reset cluster_size: %v", err)
	}

	reset, found := findClusterSizeConfigValue(after)
	if !found {
		t.Fatal("expected cluster_size to still be present after reset")
	}
	if reset.RawValue != defaultRawValue {
		t.Fatalf("expected cluster_size to be reset to %q, got %q", defaultRawValue, reset.RawValue)
	}
}

func TestResetDeploymentConfiguration_ResetAllRestoresEveryDefault(t *testing.T) {
	t.Parallel()

	deployment := newInitializedRealDeployment(t)

	before, err := GetDeploymentConfiguration(context.Background(), deployment, nil)
	if err != nil {
		t.Fatalf("failed to read initial configuration: %v", err)
	}
	clusterSize, found := findClusterSizeConfigValue(before)
	if !found {
		t.Fatal("expected the aws preset to declare a cluster_size variable")
	}
	defaultRawValue := clusterSize.RawDefault

	overrideValue := "5"
	if defaultRawValue == overrideValue {
		overrideValue = "6"
	}
	if _, err := SetDeploymentConfiguration(
		context.Background(),
		map[string]string{"cluster_size": overrideValue},
		nil,
		deployment,
	); err != nil {
		t.Fatalf("failed to override cluster_size: %v", err)
	}

	after, err := ResetDeploymentConfiguration(context.Background(), deployment, nil, true)
	if err != nil {
		t.Fatalf("failed to reset all configuration: %v", err)
	}

	reset, found := findClusterSizeConfigValue(after)
	if !found {
		t.Fatal("expected cluster_size to still be present after reset")
	}
	if reset.RawValue != defaultRawValue {
		t.Fatalf("expected cluster_size to be reset to %q, got %q", defaultRawValue, reset.RawValue)
	}
}
