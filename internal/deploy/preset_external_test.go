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

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

func TestResolvePreset_FileDirectory(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	manifestPath := filepath.Join(presetDir, presets.InfrastructureManifestFilename)
	if err := os.WriteFile(manifestPath, []byte("kind: infrastructure"), 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	manager := runtimeartifacts.NewResourceManagerForPlatform(
		runtimeartifacts.ResourceSpec{},
		t.TempDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)
	path, err := ResolvePreset(
		context.Background(),
		manager,
		"file://"+presetDir,
		presets.PresetTypeInfrastructure,
	)
	if err != nil {
		t.Fatalf("expected resolution to succeed, got %v", err)
	}
	// FileSource returns the source directory path as a redirect.
	resolved, resErr := filepath.EvalSymlinks(path)
	if resErr != nil {
		t.Fatalf("expected path to be resolvable, got %v", resErr)
	}
	wantResolved, resErr := filepath.EvalSymlinks(presetDir)
	if resErr != nil {
		t.Fatalf("expected presetDir to be resolvable, got %v", resErr)
	}
	if resolved != wantResolved {
		t.Fatalf("expected path to resolve to preset directory, got %q", resolved)
	}
}

func TestResolvePreset_FileDirectoryMissingManifestReturnsError(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()

	manager := runtimeartifacts.NewResourceManagerForPlatform(
		runtimeartifacts.ResourceSpec{},
		t.TempDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)
	_, err := ResolvePreset(
		context.Background(),
		manager,
		"file://"+presetDir,
		presets.PresetTypeInfrastructure,
	)
	if err == nil || !strings.Contains(err.Error(), "does not contain the expected") {
		t.Fatalf("expected manifest-missing error, got %v", err)
	}
}
