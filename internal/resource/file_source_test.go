// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSource_CanFetch_FileURLs(t *testing.T) {
	t.Parallel()

	src := FileSource{}
	trueURLs := []string{
		"file:///tmp/some/path",
		"file:///tmp/preset.tar.gz",
	}
	for _, url := range trueURLs {
		if !src.Handles(Locator{URL: url}) {
			t.Errorf("CanFetch(%q) = false, want true", url)
		}
	}
}

func TestFileSource_CanFetch_LocalPaths(t *testing.T) {
	t.Parallel()

	src := FileSource{}
	trueURLs := []string{
		"/tmp/some/local/path",
		"relative/path",
		"./some/file",
	}
	for _, url := range trueURLs {
		if !src.Handles(Locator{URL: url}) {
			t.Errorf("CanFetch(%q) = false, want true", url)
		}
	}
}

func TestFileSource_CanFetch_Exclusions(t *testing.T) {
	t.Parallel()

	src := FileSource{}
	falseURLs := []string{
		"git@github.com:org/repo.git",
		"https://example.com/archive.tar.gz",
		"http://example.com/archive.tar.gz",
		"git://github.com/org/repo.git",
	}
	for _, url := range falseURLs {
		if src.Handles(Locator{URL: url}) {
			t.Errorf("CanFetch(%q) = true, want false", url)
		}
	}
}

func TestFileSource_Probe_DirectoryReportsItInPlace(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "link")
	src := FileSource{}

	probe, err := src.Probe(context.Background(), Locator{URL: "file://" + srcDir})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !filepath.IsAbs(probe.Local) {
		t.Fatalf("expected absolute local path, got %q", probe.Local)
	}
	if probe.Local != srcDir {
		t.Fatalf("expected local path %q, got %q", srcDir, probe.Local)
	}
	if _, err := os.Lstat(dstDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected nothing written elsewhere, got %v", err)
	}
}

func TestFileSource_Probe_ArchiveReportsItInPlace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"preset.tar.gz", "tool.zip"} {
		filePath := filepath.Join(dir, name)
		if err := os.WriteFile(filePath, []byte("content"), filePerm); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		src := FileSource{}

		probe, err := src.Probe(context.Background(), Locator{URL: "file://" + filePath})
		if err != nil {
			t.Fatalf("Probe(%s) unexpected error: %v", name, err)
		}
		if probe.Local != filePath {
			t.Fatalf("Probe(%s) local %q does not match source %q", name, probe.Local, filePath)
		}
	}
}

func TestFileSource_Probe_BareFileReportsItInPlace(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "launcher")
	if err := os.WriteFile(filePath, []byte("content"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	src := FileSource{}

	probe, err := src.Probe(context.Background(), Locator{URL: "file://" + filePath})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if probe.Local != filePath {
		t.Fatalf("expected local path %q, got %q", filePath, probe.Local)
	}
}

func TestFileSource_Probe_MissingPathReturnsError(t *testing.T) {
	t.Parallel()

	src := FileSource{}

	_, err := src.Probe(context.Background(), Locator{URL: "file:///nonexistent/path"})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileSource_Probe_DirectoryIdentityIsStable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := FileSource{}

	first, err := src.Probe(context.Background(), Locator{URL: "file://" + dir})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if first.Identity == "" {
		t.Fatal("expected non-empty identity for directory")
	}
	second, err := src.Probe(context.Background(), Locator{URL: "file://" + dir})
	if err != nil {
		t.Fatalf("second probe: %v", err)
	}
	if first.Identity != second.Identity {
		t.Fatalf("expected stable identity, got %q then %q", first.Identity, second.Identity)
	}
}

func TestFileSource_Probe_FileReportsIdentity(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(filePath, []byte("data"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	src := FileSource{}

	probe, err := src.Probe(context.Background(), Locator{URL: "file://" + filePath})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if probe.Identity == "" {
		t.Fatal("expected non-empty identity for file")
	}
}

func TestFileSource_Probe_ChangedArchiveChangesIdentity(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "preset.tar.gz")
	if err := os.WriteFile(archive, []byte("first"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	src := FileSource{}
	loc := Locator{URL: "file://" + archive}

	before, err := src.Probe(context.Background(), loc)
	if err != nil {
		t.Fatalf("first probe: %v", err)
	}

	// Changing size avoids coarse filesystem timestamp resolution.
	if err := os.WriteFile(archive, []byte("second and longer"), filePerm); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	after, err := src.Probe(context.Background(), loc)
	if err != nil {
		t.Fatalf("second probe: %v", err)
	}
	if before.Identity == after.Identity {
		t.Fatalf("expected identity to change, both were %q", before.Identity)
	}
}

func TestFileSource_Probe_UnchangedArchiveKeepsIdentity(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "preset.tar.gz")
	if err := os.WriteFile(archive, []byte("stable"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	src := FileSource{}
	loc := Locator{URL: "file://" + archive}

	first, err := src.Probe(context.Background(), loc)
	if err != nil {
		t.Fatalf("first probe: %v", err)
	}
	second, err := src.Probe(context.Background(), loc)
	if err != nil {
		t.Fatalf("second probe: %v", err)
	}
	if first.Identity != second.Identity {
		t.Fatalf("identity %q then %q", first.Identity, second.Identity)
	}
}
