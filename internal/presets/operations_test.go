// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package presets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/resource"
	"github.com/exasol/exasol-personal/internal/resource/resourcetest"
)

func testManagerContext(t *testing.T, spec resource.ResourceSpec) context.Context {
	t.Helper()

	return resourcetest.NewManagerContext(t, spec)
}

func TestPresetDir_UnknownInfrastructureNameReturnsErrUnknownInfrastructure(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testManagerContext(t, resource.ResourceSpec{})

	// When
	_, err := presetDir(ctx, Infrastructure, "this-preset-does-not-exist")
	// Then
	if !errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected ErrUnknownInfrastructure, got %v", err)
	}
}

func TestPresetDir_UnknownInstallationNameReturnsErrUnknownInstallation(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testManagerContext(t, resource.ResourceSpec{})

	// When
	_, err := presetDir(ctx, Installation, "this-preset-does-not-exist")
	// Then
	if !errors.Is(err, ErrUnknownInstallation) {
		t.Fatalf("expected ErrUnknownInstallation, got %v", err)
	}
}

func TestListEmbeddedPresets_ReturnsNamesDeclaredUnderKind(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testManagerContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws":   memberDef(t.TempDir()),
		infrastructurePresetsResource + "/azure": memberDef(t.TempDir()),
	})

	// When
	names := ListEmbeddedPresets(ctx, Infrastructure)
	// Then
	if len(names) != 2 || names[0] != "aws" || names[1] != "azure" {
		t.Fatalf("expected [aws azure], got %v", names)
	}
}

func TestListEmbeddedPresets_EmptyForKindWithNoMembers(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testManagerContext(t, resource.ResourceSpec{})

	// When
	names := ListEmbeddedPresets(ctx, Infrastructure)
	// Then
	if len(names) != 0 {
		t.Fatalf("expected no preset names, got %v", names)
	}
}

func TestWriteDir_NamedPresetCopiesResolvedDirectory(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "aws"), 0o700); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(srcDir, "aws", "infrastructure.yaml"),
		[]byte("content"),
		0o600,
	); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	ctx := testManagerContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws": memberDef(filepath.Join(srcDir, "aws")),
	})
	outDir := filepath.Join(t.TempDir(), "out")

	// When
	err := WriteDir(ctx, Infrastructure, PresetRef{Name: "aws"}, outDir)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "infrastructure.yaml")); err != nil {
		t.Fatalf("expected copied file to exist, got %v", err)
	}
}

func TestWriteDir_UnknownNamedPresetReturnsErrUnknownInfrastructure(t *testing.T) {
	t.Parallel()

	// Given
	ctx := testManagerContext(t, resource.ResourceSpec{})

	// When
	err := WriteDir(ctx, Infrastructure, PresetRef{Name: "does-not-exist"}, t.TempDir())
	// Then
	if !errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected ErrUnknownInfrastructure, got %v", err)
	}
}

func TestWriteDir_PathPresetCopiesDirectoryDirectly(t *testing.T) {
	t.Parallel()

	// Given: a filesystem-path preset, never declared in the catalog.
	srcDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(srcDir, "installation.yaml"),
		[]byte("content"),
		0o600,
	); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	ctx := testManagerContext(t, resource.ResourceSpec{})
	outDir := filepath.Join(t.TempDir(), "out")

	// When
	err := WriteDir(ctx, Installation, PresetRef{Path: srcDir}, outDir)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "installation.yaml")); err != nil {
		t.Fatalf("expected copied file to exist, got %v", err)
	}
}

func TestWriteDir_ResolutionFailurePropagatesInsteadOfReportingUnknownPreset(t *testing.T) {
	t.Parallel()

	// Given: a declared member whose source can never resolve — not a name the
	// specification does not declare at all.
	ctx := testManagerContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws": memberDef(
			filepath.Join(t.TempDir(), "does-not-exist"),
		),
	})

	// When
	err := WriteDir(ctx, Infrastructure, PresetRef{Name: "aws"}, t.TempDir())
	// Then: the real resolution failure surfaces, not the unknown-preset
	// sentinel, which would misreport a resolvable preset as nonexistent.
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	if errors.Is(err, ErrUnknownInfrastructure) {
		t.Fatalf("expected the underlying resolution error, got the unknown-preset sentinel: %v",
			err)
	}
}

func TestReadInfrastructureFile_RejectsPathEscapingThePresetDirectory(t *testing.T) {
	t.Parallel()

	// Given: a preset directory alongside a file outside it that a
	// traversing relPath could otherwise reach.
	presetsRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(presetsRoot, "aws"), 0o700); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(presetsRoot, "secret.txt"), []byte("outside the preset"), 0o600,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	ctx := testManagerContext(t, resource.ResourceSpec{
		infrastructurePresetsResource + "/aws": memberDef(filepath.Join(presetsRoot, "aws")),
	})

	// When
	_, err := ReadInfrastructureFile(ctx, "aws", "../secret.txt")
	// Then
	if err == nil {
		t.Fatal("expected an error for a relPath escaping the preset directory, got none")
	}
}

// The assets every deployment shares are distributed like any other resource,
// and must still arrive in the deployment directory.
func TestWriteSharedDir_WritesSharedAssets(t *testing.T) {
	t.Parallel()

	ctx := resourcetest.NewContext(t)
	outDir := t.TempDir()

	if err := WriteSharedDir(ctx, outDir); err != nil {
		t.Fatalf("WriteSharedDir: %v", err)
	}

	for _, name := range []string{"eula.txt", "sample.sql"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected %s in the deployment directory: %v", name, err)
		}
	}
}

// A group member is a resource in its own right, named "<group>/<member>".
func memberDef(dir string) resource.ResourceDefinition {
	return resource.ResourceDefinition{
		Artifact: map[string]resource.ArtifactSpec{"any": {URL: dir}},
	}
}
