// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestRequireEmptyDir_ErrOnMissingDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	target := filepath.Join(base, "new")

	err := requireEmptyDir(target)
	if err == nil {
		t.Fatal("expected error")
	}
	// Must not create anything implicitly.
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected directory to not exist, got stat err: %v", statErr)
	}
}

func TestRequireEmptyDir_ErrOnNonEmptyDir(t *testing.T) {
	t.Parallel()

	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	err := requireEmptyDir(target)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExportEmbeddedPreset_Infrastructure_WritesManifest(t *testing.T) {
	t.Parallel()

	ids := presets.ListEmbeddedInfrastructuresPresets()
	if len(ids) == 0 {
		t.Skip("no embedded infrastructure presets available")
	}

	base := t.TempDir()
	target := filepath.Join(base, "infra")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := requireEmptyDir(target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h, err := resolveEmbeddedPresetHandler(ids[0], presets.PresetTypeInfrastructure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	presetRef := presets.PresetRef{Name: ids[0]}
	if err := presets.ExtractPreset(presetRef, target, h.WriteDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = os.Stat(filepath.Join(target, presets.InfrastructureManifestFilename))
	if err != nil {
		t.Fatalf("expected manifest to exist: %v", err)
	}
}

func TestExportEmbeddedPreset_Installation_WritesManifest(t *testing.T) {
	t.Parallel()

	ids := presets.ListEmbeddedInstallationsPresets()
	if len(ids) == 0 {
		t.Skip("no embedded installation presets available")
	}

	base := t.TempDir()
	target := filepath.Join(base, "install")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := requireEmptyDir(target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler, err := resolveEmbeddedPresetHandler(ids[0], presets.PresetTypeInstallation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	presetRef := presets.PresetRef{Name: ids[0]}
	if err := presets.ExtractPreset(presetRef, target, handler.WriteDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	manifestPath := filepath.Join(target, presets.InstallationManifestFilename)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest to exist: %v", err)
	}
}
