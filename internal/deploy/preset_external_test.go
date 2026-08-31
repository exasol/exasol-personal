// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/resource"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestResolvePreset_LocalGitWorktreeWithRef(t *testing.T) {
	t.Parallel()

	// Given
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("initialize repository: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	manifestPath := filepath.Join(repoDir, presets.InfrastructureManifestFilename)
	commit := func(content string) {
		t.Helper()
		if err := os.WriteFile(manifestPath, []byte(content), 0o600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		if _, err := worktree.Add(presets.InfrastructureManifestFilename); err != nil {
			t.Fatalf("stage manifest: %v", err)
		}
		_, err := worktree.Commit("preset", &git.CommitOptions{Author: &object.Signature{
			Name: "Test", Email: "test@example.com", When: time.Unix(1, 0),
		}})
		if err != nil {
			t.Fatalf("commit manifest: %v", err)
		}
	}
	commit("name: main\n")
	if err := worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"), Create: true,
	}); err != nil {
		t.Fatalf("create feature branch: %v", err)
	}
	commit("name: feature\n")
	if err := worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
	}); err != nil {
		t.Fatalf("restore default branch: %v", err)
	}
	manager := resource.NewResourceManagerForPlatform(
		resource.ResourceSpec{}, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)

	// When
	path, err := ResolvePreset(
		context.Background(), manager, repoDir+"@feature", presets.PresetTypeInfrastructure,
	)
	if err != nil {
		t.Fatalf("resolve preset: %v", err)
	}

	// Then
	manifest, err := os.ReadFile(filepath.Join(path, presets.InfrastructureManifestFilename))
	if err != nil {
		t.Fatalf("read resolved manifest: %v", err)
	}
	if string(manifest) != "name: feature\n" {
		t.Fatalf("manifest = %q, want feature branch", manifest)
	}
}

func TestResolvePreset_FileDirectory(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	manifestPath := filepath.Join(presetDir, presets.InfrastructureManifestFilename)
	if err := os.WriteFile(manifestPath, []byte("kind: infrastructure"), 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	manager := resource.NewResourceManagerForPlatform(
		resource.ResourceSpec{},
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

	manager := resource.NewResourceManagerForPlatform(
		resource.ResourceSpec{},
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

func TestResolvePreset_FileDirectoryWithFragmentResolvesSubdirectory(t *testing.T) {
	t.Parallel()

	// Given — a preset root containing a subdirectory with the manifest.
	presetRoot := t.TempDir()
	subDir := filepath.Join(presetRoot, "infra", "aws")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatalf("failed to create sub directory: %v", err)
	}
	manifestPath := filepath.Join(subDir, presets.InfrastructureManifestFilename)
	if err := os.WriteFile(manifestPath, []byte("kind: infrastructure"), 0o600); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	manager := resource.NewResourceManagerForPlatform(
		resource.ResourceSpec{},
		t.TempDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)
	// When
	path, err := ResolvePreset(
		context.Background(),
		manager,
		"file://"+presetRoot+"#infra/aws",
		presets.PresetTypeInfrastructure,
	)
	// Then
	if err != nil {
		t.Fatalf("expected resolution to succeed, got %v", err)
	}
	if path != subDir {
		t.Fatalf("expected path %q, got %q", subDir, path)
	}
}

func TestResolvePreset_FileArchiveWithFragmentResolvesSubdirectory(t *testing.T) {
	t.Parallel()

	// Given — an archive that contains multiple presets under distinct subpaths.
	fixtureDir := t.TempDir()
	archivePath := writePresetTarGz(t, fixtureDir, "presets.tar.gz", map[string]string{
		"infra/aws/" + presets.InfrastructureManifestFilename:         "name: aws",
		"infra/azure/" + presets.InfrastructureManifestFilename:       "name: azure",
		"installation/ubuntu/" + presets.InstallationManifestFilename: "name: ubuntu",
	})

	manager := resource.NewResourceManagerForPlatform(
		resource.ResourceSpec{},
		t.TempDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)

	// When — resolve the "aws" preset via a fragment
	path, err := ResolvePreset(
		context.Background(),
		manager,
		"file://"+archivePath+"#infra/aws",
		presets.PresetTypeInfrastructure,
	)
	// Then
	if err != nil {
		t.Fatalf("expected resolution to succeed, got %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("infra", "aws")) {
		t.Fatalf("expected resolved path to end with infra/aws, got %q", path)
	}
	if _, err := os.Stat(filepath.Join(path, presets.InfrastructureManifestFilename)); err != nil {
		t.Fatalf("expected manifest in resolved subdirectory, got %v", err)
	}
}

func TestResolvePreset_FileArchiveFragmentPointingAtNonPresetSubdirReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a subdir exists but lacks the required manifest.
	fixtureDir := t.TempDir()
	archivePath := writePresetTarGz(t, fixtureDir, "presets.tar.gz", map[string]string{
		"infra/aws/README.md": "no manifest here",
	})

	manager := resource.NewResourceManagerForPlatform(
		resource.ResourceSpec{},
		t.TempDir(),
		runtime.GOOS,
		runtime.GOARCH,
	)

	// When
	_, err := ResolvePreset(
		context.Background(),
		manager,
		"file://"+archivePath+"#infra/aws",
		presets.PresetTypeInfrastructure,
	)
	// Then
	if err == nil || !strings.Contains(err.Error(), "does not contain the expected") {
		t.Fatalf("expected manifest-missing error, got %v", err)
	}
}

// writePresetTarGz creates a .tar.gz with the given path→content entries and
// returns the archive path. Only regular-file headers are written; nested
// paths rely on the extractor creating parent directories on demand.
func writePresetTarGz(t *testing.T, dir, name string, entries map[string]string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	outputFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	gzipWriter := gzip.NewWriter(outputFile)
	tarWriter := tar.NewWriter(gzipWriter)

	for entryName, content := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{
			Name:     entryName,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("tar header %q: %v", entryName, err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("tar payload %q: %v", entryName, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := outputFile.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	return path
}
