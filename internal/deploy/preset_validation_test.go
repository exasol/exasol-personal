// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestValidatePresetSelection_AcceptsDefaultEmbeddedPair(t *testing.T) {
	t.Parallel()

	// Given
	infrastructurePreset := PresetRef{Name: presets.DefaultInfrastructure}
	installationPreset := PresetRef{Name: presets.DefaultInstallation}

	// When
	err := validatePresetSelection(infrastructurePreset, installationPreset)

	// Then
	if err != nil {
		t.Fatalf("expected default preset pair to be valid, got %v", err)
	}
}

func TestInitDeployment_RejectsIncompatiblePresetPairBeforeMutation(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	infrastructureDir := t.TempDir()
	installationDir := t.TempDir()

	writeTestFile(t, filepath.Join(infrastructureDir, presets.InfrastructureManifestFilename), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
compatibility:
  provides:
    - local-command
`)
	writeTestFile(t, filepath.Join(installationDir, presets.InstallationManifestFilename), `
name: Test Installation
description: test installation
compatibility:
  requires:
    - remote-exec
install: []
`)

	// When
	err := InitDeployment(
		context.Background(),
		PresetRef{Path: infrastructureDir},
		PresetRef{Path: installationDir},
		map[string]string{},
		map[string]string{},
		config.NewDeploymentDir(deploymentDir),
		false,
		"0.0.0",
	)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing capabilities [remote-exec]") {
		t.Fatalf("expected compatibility error, got %v", err)
	}

	entries, readErr := os.ReadDir(deploymentDir)
	if readErr != nil {
		t.Fatalf("expected to read deployment dir, got %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected deployment directory to remain untouched, found %d entries", len(entries))
	}
}

func TestValidatePresetSelection_RejectsLocalPresetOnUnsupportedPlatform(t *testing.T) {
	// Given
	originalPlatformSupport := localRuntimePlatformSupported
	localRuntimePlatformSupported = func() bool { return false }
	defer func() { localRuntimePlatformSupported = originalPlatformSupport }()

	infrastructurePreset := PresetRef{Name: "local"}
	installationPreset := PresetRef{Name: presets.DefaultLocalInstallation}

	// When
	err := validatePresetSelection(infrastructurePreset, installationPreset)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Apple Silicon macOS") {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

func TestResolveDefaultInstallationPreset_LocalUsesNano(t *testing.T) {
	t.Parallel()

	// Given
	infrastructurePreset := PresetRef{Name: "local"}

	// When
	installationPreset, err := ResolveDefaultInstallationPreset(infrastructurePreset)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if installationPreset.Name != presets.DefaultLocalInstallation {
		t.Fatalf("expected %q, got %#v", presets.DefaultLocalInstallation, installationPreset)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}
