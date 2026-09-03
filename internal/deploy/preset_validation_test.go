// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/resource"
	"github.com/exasol/exasol-personal/internal/resource/resourcetest"
)

func TestValidatePresetSelection_AcceptsDefaultEmbeddedPair(t *testing.T) {
	t.Parallel()

	// Given
	infrastructurePreset := PresetRef{Name: presets.DefaultInfrastructure}
	installationPreset := PresetRef{Name: presets.DefaultInstallation}

	// When
	err := ValidatePresetSelection(testResolverContext(t), infrastructurePreset, installationPreset)
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
		testResolverContext(t),
		config.NewDeploymentDir(deploymentDir),
		InitOptions{
			InfrastructurePreset: PresetRef{Path: infrastructureDir},
			InstallationPreset:   PresetRef{Path: installationDir},
			InfraVars:            map[string]string{},
			InstallVars:          map[string]string{},
			CurrentVersion:       "0.0.0",
		},
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

func TestInitDeploymentFetchesInstallationPresetFromLauncherResources(t *testing.T) {
	t.Parallel()

	// Given
	infrastructureDir := t.TempDir()
	installationDir := t.TempDir()
	wrongInfrastructureDir := t.TempDir()
	wrongInstallationDir := t.TempDir()
	writeTestFile(t, filepath.Join(infrastructureDir, presets.InfrastructureManifestFilename), `
name: Test Infrastructure
description: Test infrastructure
backend: tofu
`)
	writeTestFile(t, filepath.Join(installationDir, presets.InstallationManifestFilename), `
name: Launcher Installation
description: Launcher installation
install: []
`)
	writeTestFile(
		t,
		filepath.Join(wrongInstallationDir, presets.InstallationManifestFilename),
		`
name: Infrastructure Override
description: Infrastructure override
install: []
`,
	)
	writeTestFile(
		t,
		filepath.Join(wrongInfrastructureDir, presets.InfrastructureManifestFilename),
		`
name: Installation Override
description: Installation override
backend: tofu
`,
	)
	writeTestFile(t, filepath.Join(infrastructureDir, "resources.yaml"), `
installation-presets/test:
  artifact:
    any:
      url: file://`+wrongInstallationDir)
	writeTestFile(t, filepath.Join(installationDir, "resources.yaml"), `
infrastructure-presets/test:
  artifact:
    any:
      url: file://`+wrongInfrastructureDir)
	ctx := resourcetest.NewResolverContext(t, resource.ResourceSpec{
		"infrastructure-presets/test": presetDefinition(infrastructureDir),
		"installation-presets/test":   presetDefinition(installationDir),
		"shared-assets":               presetDefinition(t.TempDir()),
	})
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	err := InitDeployment(ctx, deployment, InitOptions{
		InfrastructurePreset: PresetRef{Name: "test"},
		InstallationPreset:   PresetRef{Name: "test"},
		InfraVars:            map[string]string{},
		InstallVars:          map[string]string{},
		CurrentVersion:       "0.0.0",
	})
	// Then
	if err != nil {
		t.Fatalf("initialize deployment: %v", err)
	}
	manifest, err := presets.ReadInstallManifestFromDir(deployment.InstallationDir())
	if err != nil {
		t.Fatalf("read extracted installation manifest: %v", err)
	}
	if manifest.Name != "Launcher Installation" {
		t.Fatalf("installation name = %q, want launcher preset", manifest.Name)
	}
	infrastructureManifest, err := presets.ReadInfrastructureManifestFromDir(
		deployment.InfrastructureDir(),
	)
	if err != nil {
		t.Fatalf("read extracted infrastructure manifest: %v", err)
	}
	if infrastructureManifest.Name != "Test Infrastructure" {
		t.Fatalf("infrastructure name = %q, want launcher preset", infrastructureManifest.Name)
	}
}

func presetDefinition(dir string) resource.ResourceDefinition {
	return resource.ResourceDefinition{Artifact: map[string]resource.ArtifactSpec{
		"any": {URL: "file://" + dir},
	}}
}

func TestResolveDefaultInstallationPreset_UsesCompatibleEmbeddedDefault(t *testing.T) {
	t.Parallel()

	// Given
	infrastructurePreset := PresetRef{Name: presets.DefaultInfrastructure}

	// When
	installationPreset, err := ResolveDefaultInstallationPreset(
		testResolverContext(t),
		infrastructurePreset,
	)
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
