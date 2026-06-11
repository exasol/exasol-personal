// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	err := ValidatePresetSelection(infrastructurePreset, installationPreset)
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
		t.Fatalf(
			"expected deployment directory to remain untouched, found %d entries",
			len(entries),
		)
	}
}

func TestInitDeployment_RejectsUnsupportedLocalPlatformBeforeMutation(t *testing.T) {
	if runtime.GOOS == localSupportedOS && runtime.GOARCH == localSupportedArch {
		t.Skip("current platform supports local deployments")
	}
	t.Setenv(localAllowUnsupportedEnv, "")

	// Given
	deploymentDir := t.TempDir()

	// When
	err := InitDeployment(
		context.Background(),
		PresetRef{Name: "local"},
		PresetRef{Name: "local"},
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
	if !strings.Contains(err.Error(), errUnsupportedLocalPlatform.Error()) {
		t.Fatalf("expected unsupported local platform error, got %v", err)
	}

	entries, readErr := os.ReadDir(deploymentDir)
	if readErr != nil {
		t.Fatalf("expected to read deployment dir, got %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf(
			"expected deployment directory to remain untouched, found %d entries",
			len(entries),
		)
	}
}

func TestResolveDefaultInstallationPreset_UsesCompatibleEmbeddedDefault(t *testing.T) {
	t.Parallel()

	// Given
	infrastructurePreset := PresetRef{Name: presets.DefaultInfrastructure}

	// When
	installationPreset, err := ResolveDefaultInstallationPreset(infrastructurePreset)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if installationPreset.Name != presets.DefaultInstallation {
		t.Fatalf("expected %q, got %#v", presets.DefaultInstallation, installationPreset)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}
