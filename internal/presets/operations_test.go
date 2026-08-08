// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const localPresetName = "local"

func TestPresetRefIsPath(t *testing.T) {
	t.Parallel()

	if (PresetRef{Name: "local"}).IsPath() {
		t.Fatal("expected a name-only ref to not be a path")
	}
	if !(PresetRef{Path: "/some/dir"}).IsPath() {
		t.Fatal("expected a ref with a path to be a path")
	}
	if (PresetRef{Path: "   "}).IsPath() {
		t.Fatal("expected a whitespace-only path to not count as a path")
	}
}

func TestListEmbeddedInfrastructuresPresets_ReturnsRealSortedNames(t *testing.T) {
	t.Parallel()

	names := ListEmbeddedInfrastructuresPresets()

	if len(names) == 0 {
		t.Fatal("expected at least one embedded infrastructure preset")
	}
	found := false
	for i, name := range names {
		if name == localPresetName {
			found = true
		}
		if i > 0 && names[i-1] > name {
			t.Fatalf("expected sorted names, got %v", names)
		}
	}
	if !found {
		t.Fatalf("expected 'local' among embedded infrastructure presets, got %v", names)
	}
}

func TestListEmbeddedInstallationsPresets_ReturnsRealSortedNames(t *testing.T) {
	t.Parallel()

	names := ListEmbeddedInstallationsPresets()

	if len(names) == 0 {
		t.Fatal("expected at least one embedded installation preset")
	}
	found := false
	for i, name := range names {
		if name == localPresetName {
			found = true
		}
		if i > 0 && names[i-1] > name {
			t.Fatalf("expected sorted names, got %v", names)
		}
	}
	if !found {
		t.Fatalf("expected 'local' among embedded installation presets, got %v", names)
	}
}

func TestWriteInfrastructureDir_WritesRealEmbeddedPresetContent(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	if err := WriteInfrastructureDir("local", outDir); err != nil {
		t.Fatalf("expected the embedded 'local' preset to be written, got %v", err)
	}

	manifestPath := filepath.Join(outDir, InfrastructureManifestFilename)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected infrastructure manifest to be written, got %v", err)
	}
}

func TestWriteInfrastructureDir_UnknownPresetReturnsError(t *testing.T) {
	t.Parallel()

	err := WriteInfrastructureDir("does-not-exist", t.TempDir())

	if !errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected ErrUnknownInfrastructure, got %v", err)
	}
}

func TestWriteInstallDir_WritesRealEmbeddedPresetContent(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	if err := WriteInstallDir("local", outDir); err != nil {
		t.Fatalf("expected the embedded 'local' preset to be written, got %v", err)
	}

	manifestPath := filepath.Join(outDir, InstallationManifestFilename)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected installation manifest to be written, got %v", err)
	}
}

func TestWriteInstallDir_UnknownPresetReturnsError(t *testing.T) {
	t.Parallel()

	err := WriteInstallDir("does-not-exist", t.TempDir())

	if !errors.Is(err, ErrUnknownInstallation) {
		t.Fatalf("expected ErrUnknownInstallation, got %v", err)
	}
}

func TestWriteSharedDir_WritesSharedAssetFiles(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	if err := WriteSharedDir(outDir); err != nil {
		t.Fatalf("expected shared assets to be written, got %v", err)
	}

	for _, name := range []string{"eula.txt", "sample.sql"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected shared asset %q to be written, got %v", name, err)
		}
	}
}

func TestReadInfrastructureFile_ReadsRealManifestBytes(t *testing.T) {
	t.Parallel()

	data, err := ReadInfrastructureFile("local", InfrastructureManifestFilename)
	if err != nil {
		t.Fatalf("expected the embedded manifest file to be readable, got %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty manifest content")
	}
}

func TestReadInfrastructureFile_MissingFileReturnsError(t *testing.T) {
	t.Parallel()

	_, err := ReadInfrastructureFile("local", "does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected an error for a missing embedded file")
	}
}

func TestExtractPreset_PathBasedCopiesFromDisk(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "marker.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write marker file: %v", err)
	}
	dstDir := filepath.Join(t.TempDir(), "extracted")

	writeEmbeddedCalled := false

	err := ExtractPreset(PresetRef{Path: srcDir}, dstDir, func(string, string) error {
		writeEmbeddedCalled = true

		return nil
	})
	if err != nil {
		t.Fatalf("expected path-based extraction to succeed, got %v", err)
	}
	if writeEmbeddedCalled {
		t.Fatal("expected writeEmbedded not to be called for a path-based preset")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "marker.txt")); err != nil {
		t.Fatalf("expected marker file to be copied, got %v", err)
	}
}

func TestExtractPreset_NameBasedDelegatesToWriteEmbedded(t *testing.T) {
	t.Parallel()

	var gotName, gotOutDir string

	err := ExtractPreset(PresetRef{Name: "local"}, "/some/out", func(name, outDir string) error {
		gotName = name
		gotOutDir = outDir

		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotName != "local" || gotOutDir != "/some/out" {
		t.Fatalf("expected writeEmbedded to be called with (local, /some/out), got (%s, %s)",
			gotName, gotOutDir)
	}
}
