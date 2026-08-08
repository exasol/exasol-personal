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

func TestIsExternalPresetURI(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"file:///tmp/preset":               true,
		"http://example.com/preset.tar.gz": true,
		"https://example.com/preset.zip":   true,
		"git://example.com/repo.git":       true,
		"git@github.com:org/repo.git":      true,
		"aws":                              false,
		"/local/path/to/preset":            false,
		"":                                 false,
	}
	for uri, want := range cases {
		if got := IsExternalPresetURI(uri); got != want {
			t.Errorf("IsExternalPresetURI(%q) = %v, want %v", uri, got, want)
		}
	}
}

func TestManifestFilenameFor(t *testing.T) {
	t.Parallel()

	gotInfra := manifestFilenameFor(presets.PresetTypeInfrastructure)
	if gotInfra != presets.InfrastructureManifestFilename {
		t.Errorf("expected infrastructure manifest filename, got %q", gotInfra)
	}
	gotInstall := manifestFilenameFor(presets.PresetTypeInstallation)
	if gotInstall != presets.InstallationManifestFilename {
		t.Errorf("expected installation manifest filename, got %q", gotInstall)
	}
	if got := manifestFilenameFor("unknown"); got != "" {
		t.Errorf("expected empty filename for unknown preset type, got %q", got)
	}
}

func TestNeedsExtraction(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"https://example.com/preset.tar.gz": true,
		"https://example.com/preset.tgz":    true,
		"https://example.com/preset.zip":    true,
		"https://example.com/preset":        false,
		"file:///tmp/preset-dir":            false,
	}
	for url, want := range cases {
		if got := needsExtraction(url); got != want {
			t.Errorf("needsExtraction(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestResolvePreset_NonGitURLWithRefReturnsError(t *testing.T) {
	t.Parallel()

	_, err := ResolvePreset(
		context.Background(),
		nil,
		"https://example.com/preset.tar.gz@something",
		presets.PresetTypeInfrastructure,
	)
	if err == nil || !strings.Contains(err.Error(), "@ref syntax") {
		t.Fatalf("expected @ref error for non-git URL, got %v", err)
	}
}

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
	if path != presetDir {
		t.Fatalf("expected path to be preset directory %q, got %q", presetDir, path)
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
