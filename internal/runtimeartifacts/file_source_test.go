// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

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
		if !src.CanFetch(url) {
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
		if !src.CanFetch(url) {
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
		if src.CanFetch(url) {
			t.Errorf("CanFetch(%q) = true, want false", url)
		}
	}
}

func TestFileSource_Fetch_DirectoryReturnsRedirectPath(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "link")
	src := FileSource{}

	redirectPath, err := src.Fetch(context.Background(), "file://"+srcDir, dstDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !filepath.IsAbs(redirectPath) {
		t.Fatalf("expected absolute redirect path, got %q", redirectPath)
	}
	if redirectPath != srcDir {
		t.Fatalf("expected redirect to %q, got %q", srcDir, redirectPath)
	}
	// Nothing written to dstDir.
	if _, err := os.Lstat(dstDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected nothing at dstDir, got %v", err)
	}
}

func TestFileSource_Fetch_ArchiveReturnsRedirectPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cases := []string{"preset.tar.gz", "tool.zip"}
	for _, name := range cases {
		filePath := filepath.Join(dir, name)
		if err := os.WriteFile(filePath, []byte("content"), filePerm); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		src := FileSource{}

		redirectPath, err := src.Fetch(context.Background(), "file://"+filePath, "ignored")
		if err != nil {
			t.Fatalf("Fetch(%s) unexpected error: %v", name, err)
		}
		if redirectPath == "" {
			t.Fatalf("Fetch(%s) expected non-empty redirect path", name)
		}
		if redirectPath != filePath {
			t.Fatalf("Fetch(%s) redirect %q does not match source %q", name, redirectPath, filePath)
		}
	}
}

func TestFileSource_Fetch_UnsupportedFileTypeReturnsError(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filePath, []byte("content"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	src := FileSource{}

	_, err := src.Fetch(context.Background(), "file://"+filePath, "ignored")
	const wantMsg = "must be a directory or a supported archive file"
	if err == nil || !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("expected unsupported-type error, got %v", err)
	}
}

func TestFileSource_Fetch_MissingPathReturnsError(t *testing.T) {
	t.Parallel()

	dstPath := filepath.Join(t.TempDir(), "dst")
	src := FileSource{}

	_, err := src.Fetch(context.Background(), "file:///nonexistent/path", dstPath)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileSource_Identify_DirectoryReturnsHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := FileSource{}

	hash, err := src.Identify(context.Background(), "file://"+dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash for directory")
	}
	// Same path always produces the same hash.
	hash2, err := src.Identify(context.Background(), "file://"+dir)
	if err != nil {
		t.Fatalf("second Identify: %v", err)
	}
	if hash != hash2 {
		t.Fatalf("expected stable hash, got %q then %q", hash, hash2)
	}
}

func TestFileSource_Identify_FileReturnsPathHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(filePath, []byte("data"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	src := FileSource{}

	hash, err := src.Identify(context.Background(), "file://"+filePath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash for file")
	}
}

func TestFileSource_Identify_MissingPathReturnsError(t *testing.T) {
	t.Parallel()

	src := FileSource{}

	_, err := src.Identify(context.Background(), "file:///nonexistent/identify/path")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}
