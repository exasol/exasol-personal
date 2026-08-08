// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"errors"
	"os"
	"path/filepath"
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

func TestReadInfrastructureManifestFromDir_MissingFileReturnsWrappedError(t *testing.T) {
	t.Parallel()

	// Given / When
	_, err := ReadInfrastructureManifestFromDir(t.TempDir())
	// Then
	if err == nil {
		t.Fatal("expected an error for a missing manifest file")
	}
}

func TestReadInfrastructureManifestFromDir_ParsesRealManifestFile(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	manifestYAML := "name: Test Infra\ndescription: test infra\nbackend: local\n"
	path := filepath.Join(dir, InfrastructureManifestFilename)
	if err := os.WriteFile(path, []byte(manifestYAML), 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// When
	manifest, err := ReadInfrastructureManifestFromDir(dir)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if manifest.Name != "Test Infra" {
		t.Fatalf("expected manifest name to be parsed, got %q", manifest.Name)
	}
}

func TestParseInfrastructureManifest_RejectsMissingName(t *testing.T) {
	t.Parallel()

	// Given / When
	_, err := parseInfrastructureManifest([]byte("description: test infra\n"))

	// Then
	if !errors.Is(err, ErrMissingName) {
		t.Fatalf("expected ErrMissingName, got %v", err)
	}
}

func TestParseInfrastructureManifest_RejectsMissingDescription(t *testing.T) {
	t.Parallel()

	// Given / When
	_, err := parseInfrastructureManifest([]byte("name: Test\n"))

	// Then
	if !errors.Is(err, ErrMissingDescription) {
		t.Fatalf("expected ErrMissingDescription, got %v", err)
	}
}

func TestReadInfrastructureManifest_UnknownPresetReturnsError(t *testing.T) {
	t.Parallel()

	// Given / When
	_, err := ReadInfrastructureManifest("does-not-exist")
	// Then
	if err == nil {
		t.Fatal("expected an error for an unknown infrastructure preset")
	}
}
