// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestTarGzExtractor_CanExtract(t *testing.T) {
	t.Parallel()

	ext := &TarGzExtractor{}
	trueNames := []string{
		"archive.tar.gz",
		"tool-linux-amd64.tgz",
		"/absolute/path/preset.tar.gz",
	}
	for _, name := range trueNames {
		if !ext.CanExtract(name) {
			t.Errorf("CanExtract(%q) = false, want true", name)
		}
	}

	falseNames := []string{
		"archive.zip",
		"archive.tar",
		"archive.gz",
		"preset.tar.gz.bak",
		"",
	}
	for _, name := range falseNames {
		if ext.CanExtract(name) {
			t.Errorf("CanExtract(%q) = true, want false", name)
		}
	}
}

func TestTarGzExtractor_Extract_SingleFile(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	archivePath := writeTarGzFixture(t, srcDir, "archive.tar.gz", "tool")
	dstDir := t.TempDir()
	ext := &TarGzExtractor{}

	if err := ext.Extract(archivePath, dstDir); err != nil {
		t.Fatalf("expected extraction to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "tool")); err != nil {
		t.Fatalf("expected extracted file to exist, got %v", err)
	}
}

func TestTarGzExtractor_Extract_MultipleFiles(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(t, srcDir, "archive.tar.gz", map[string]tarFixtureEntry{
		"bin/tool":  {Content: "binary", Mode: 0o755},
		"README.md": {Content: "readme", Mode: 0o644},
		"sub/":      {IsDir: true, Mode: 0o755},
		"sub/cfg":   {Content: "config", Mode: 0o644},
	})
	dstDir := t.TempDir()
	ext := &TarGzExtractor{}

	if err := ext.Extract(archivePath, dstDir); err != nil {
		t.Fatalf("expected extraction to succeed, got %v", err)
	}
	for _, rel := range []string{"bin/tool", "README.md", "sub/cfg"} {
		if _, err := os.Stat(filepath.Join(dstDir, rel)); err != nil {
			t.Errorf("expected %q to exist after extraction, got %v", rel, err)
		}
	}
}

func TestTarGzExtractor_Extract_PermissionsPreserved(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(t, srcDir, "archive.tar.gz", map[string]tarFixtureEntry{
		"tool": {Content: "binary", Mode: 0o750},
	})
	dstDir := t.TempDir()
	ext := &TarGzExtractor{}

	if err := ext.Extract(archivePath, dstDir); err != nil {
		t.Fatalf("expected extraction to succeed, got %v", err)
	}
	info, err := os.Stat(filepath.Join(dstDir, "tool"))
	if err != nil {
		t.Fatalf("expected tool to exist, got %v", err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("expected mode 0750, got %v", info.Mode().Perm())
	}
}

func TestTarGzExtractor_Extract_RejectsPathTraversal(t *testing.T) {
	t.Parallel()

	archivePath := writeTarGzWithEntry(t, "../evil", "evil content")
	dstDir := t.TempDir()
	ext := &TarGzExtractor{}

	if err := ext.Extract(archivePath, dstDir); err == nil {
		t.Fatal("expected path-traversal error, got nil")
	}
}

func writeTarGzWithEntry(t *testing.T, entryName, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	archiveFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	gzw := gzip.NewWriter(archiveFile)
	tarWriter := tar.NewWriter(gzw)

	payload := []byte(content)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name:     entryName,
		Mode:     0o644,
		Size:     int64(len(payload)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tarWriter.Write(payload); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	return path
}
