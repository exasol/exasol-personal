// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestZipExtractor_CanExtract(t *testing.T) {
	t.Parallel()

	ext := &ZipExtractor{}
	trueNames := []string{
		"archive.zip",
		"preset-linux.zip",
		"/absolute/path/archive.zip",
	}
	for _, name := range trueNames {
		if !ext.CanExtract(name) {
			t.Errorf("CanExtract(%q) = false, want true", name)
		}
	}

	falseNames := []string{
		"archive.tar.gz",
		"archive.tgz",
		"archive.tar",
		"preset.zip.bak",
		"",
	}
	for _, name := range falseNames {
		if ext.CanExtract(name) {
			t.Errorf("CanExtract(%q) = true, want false", name)
		}
	}
}

func TestZipExtractor_Extract_SingleFile(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	archivePath := writeZipSingleFixture(t, srcDir, "archive.zip", "tool", "tool-content")
	dstDir := t.TempDir()
	ext := &ZipExtractor{}

	if err := ext.Extract(archivePath, dstDir); err != nil {
		t.Fatalf("expected extraction to succeed, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dstDir, "tool"))
	if err != nil {
		t.Fatalf("expected extracted file to exist, got %v", err)
	}
	if string(data) != "tool-content" {
		t.Fatalf("expected content %q, got %q", "tool-content", string(data))
	}
}

func TestZipExtractor_Extract_MultipleFiles(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	archivePath := writeZipMultiFixture(t, srcDir, "archive.zip", map[string]string{
		"bin/tool":  "binary",
		"README.md": "readme",
		"sub/cfg":   "config",
	})
	dstDir := t.TempDir()
	ext := &ZipExtractor{}

	if err := ext.Extract(archivePath, dstDir); err != nil {
		t.Fatalf("expected extraction to succeed, got %v", err)
	}
	for _, rel := range []string{"bin/tool", "README.md", "sub/cfg"} {
		if _, err := os.Stat(filepath.Join(dstDir, rel)); err != nil {
			t.Errorf("expected %q to exist after extraction, got %v", rel, err)
		}
	}
}

func TestZipExtractor_Extract_RejectsPathTraversal(t *testing.T) {
	t.Parallel()

	archivePath := writeZipSingleFixture(t, t.TempDir(), "archive.zip", "../evil", "evil content")
	dstDir := t.TempDir()
	ext := &ZipExtractor{}

	if err := ext.Extract(archivePath, dstDir); err == nil {
		t.Fatal("expected path-traversal error, got nil")
	}
}

func writeZipMultiFixture(t *testing.T, dir, archiveName string, entries map[string]string) string {
	t.Helper()
	return writeZipFixtureEntries(t, dir, archiveName, entries)
}

func writeZipSingleFixture(t *testing.T, dir, archiveName, entryName, content string) string {
	t.Helper()
	return writeZipFixtureEntries(t, dir, archiveName, map[string]string{
		entryName: content,
	})
}
