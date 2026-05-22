// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"slices"
	"strings"
	"testing"
)

func TestBuiltInAdminUIPresetsDeclareCapability(t *testing.T) {
	t.Parallel()

	for _, presetName := range []string{"aws", "azure", "exoscale"} {
		t.Run(presetName, func(t *testing.T) {
			t.Parallel()

			// Given / When
			manifest, err := ReadInfrastructureManifest(presetName)
			// Then
			if err != nil {
				t.Fatalf("failed to read infrastructure manifest: %v", err)
			}
			if !slices.Contains(manifest.ProvidedCapabilities(), "admin-ui") {
				t.Fatalf(
					"expected %q to provide admin-ui, got %#v",
					presetName,
					manifest.ProvidedCapabilities(),
				)
			}
		})
	}
}

func TestBuiltInLocalPresetDoesNotDeclareAdminUICapability(t *testing.T) {
	t.Parallel()

	// Given / When
	manifest, err := ReadInfrastructureManifest("local")
	// Then
	if err != nil {
		t.Fatalf("failed to read local infrastructure manifest: %v", err)
	}
	if slices.Contains(manifest.ProvidedCapabilities(), "admin-ui") {
		t.Fatalf(
			"expected local preset not to provide admin-ui, got %#v",
			manifest.ProvidedCapabilities(),
		)
	}
}

func TestBuiltInCloudPresetsEmitAdminUIMetadata(t *testing.T) {
	t.Parallel()

	expectedOutputs := []string{
		"adminUi",
		"url",
		"username",
		"insecureSkipCertValidation",
	}
	for _, presetName := range []string{"aws", "azure", "exoscale"} {
		t.Run(presetName, func(t *testing.T) {
			t.Parallel()

			// Given / When
			outputs, err := ReadInfrastructureFile(presetName, "outputs.tf")
			// Then
			if err != nil {
				t.Fatalf("failed to read outputs.tf: %v", err)
			}
			outputsText := string(outputs)
			for _, expected := range expectedOutputs {
				if !strings.Contains(outputsText, expected) {
					t.Fatalf("expected %q outputs.tf to contain %q", presetName, expected)
				}
			}
		})
	}
}
