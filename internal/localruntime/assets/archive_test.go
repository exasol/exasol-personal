// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubExtractTarXz replaces the package-level extract command with a fake
// that lays down `mac-arm64/exasol-vm.img` inside outDir to mimic what tar
// would have done. Restores the original on test cleanup.
func stubExtractTarXz(t *testing.T, contents []byte) {
	t.Helper()
	original := extractTarXzCommand
	extractTarXzCommand = func(_ string, outDir string) *exec.Cmd {
		// `sh -c` is portable enough for the tests; doesn't actually depend on
		// the archive contents.
		bundleDir := filepath.Join(outDir, "mac-arm64")
		imgPath := filepath.Join(bundleDir, "exasol-vm.img")
		script := "mkdir -p '" + bundleDir + "' && printf '%s' '" +
			strings.ReplaceAll(string(contents), "'", "'\\''") +
			"' > '" + imgPath + "'"

		return exec.CommandContext(context.Background(), "sh", "-c", script)
	}
	t.Cleanup(func() {
		extractTarXzCommand = original
	})
}

func TestResolveDiskImagePath_ReturnsRawFileWhenNotArchive(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "exasol-vm.img")
	if err := os.WriteFile(rawPath, []byte("disk"), 0o600); err != nil {
		t.Fatalf("expected fixture written, got %v", err)
	}

	// When
	resolved, err := resolveDiskImagePath(rawPath)
	// Then
	if err != nil {
		t.Fatalf("expected resolveDiskImagePath to pass through, got %v", err)
	}
	if resolved != rawPath {
		t.Fatalf("expected raw path %q, got %q", rawPath, resolved)
	}
}

//nolint:paralleltest // mutates package-level extractTarXzCommand hook.
func TestResolveDiskImagePath_ExtractsTarXzAndReturnsImg(t *testing.T) {
	stubExtractTarXz(t, []byte("disk-contents"))

	// Given
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mac-arm64.tar.xz")
	if err := os.WriteFile(archivePath, []byte("archive-bytes"), 0o600); err != nil {
		t.Fatalf("expected archive fixture written, got %v", err)
	}

	// When
	resolved, err := resolveDiskImagePath(archivePath)
	// Then
	if err != nil {
		t.Fatalf("expected extraction to succeed, got %v", err)
	}
	expectedDir := filepath.Join(dir, extractedSubdir, "mac-arm64")
	expectedImg := filepath.Join(expectedDir, "exasol-vm.img")
	if resolved != expectedImg {
		t.Fatalf("expected resolved %q, got %q", expectedImg, resolved)
	}
	contents, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("expected resolved img to exist, got %v", err)
	}
	if string(contents) != "disk-contents" {
		t.Fatalf("expected stub-written contents, got %q", string(contents))
	}
}

//nolint:paralleltest // mutates package-level extractTarXzCommand hook.
func TestResolveDiskImagePath_ReusesExtractionOnSecondCall(t *testing.T) {
	stubExtractTarXz(t, []byte("disk-contents"))

	// Given
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mac-arm64.tar.xz")
	if err := os.WriteFile(archivePath, []byte("archive-bytes"), 0o600); err != nil {
		t.Fatalf("expected archive fixture written, got %v", err)
	}

	first, err := resolveDiskImagePath(archivePath)
	if err != nil {
		t.Fatalf("expected first extraction to succeed, got %v", err)
	}
	firstInfo, err := os.Stat(first)
	if err != nil {
		t.Fatalf("expected resolved img to exist after first call, got %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// When
	second, err := resolveDiskImagePath(archivePath)
	if err != nil {
		t.Fatalf("expected second resolution to succeed, got %v", err)
	}
	secondInfo, err := os.Stat(second)
	if err != nil {
		t.Fatalf("expected resolved img to still exist, got %v", err)
	}

	// Then
	if first != second {
		t.Fatalf("expected reused img path, got %q then %q", first, second)
	}
	if !secondInfo.ModTime().Equal(firstInfo.ModTime()) {
		t.Fatalf(
			"expected img mtime unchanged on reuse (%v vs %v)",
			firstInfo.ModTime(),
			secondInfo.ModTime(),
		)
	}
}

//nolint:paralleltest // mutates package-level extractTarXzCommand hook.
func TestResolveDiskImagePath_ErrorsWhenArchiveContainsNoImg(t *testing.T) {
	original := extractTarXzCommand
	extractTarXzCommand = func(_ string, outDir string) *exec.Cmd {
		// Lay down a non-img file to mimic a malformed archive.
		readme := filepath.Join(outDir, "README.md")

		return exec.CommandContext(context.Background(), "sh", "-c",
			"echo bare > '"+readme+"'")
	}
	t.Cleanup(func() { extractTarXzCommand = original })

	// Given
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "broken.tar.xz")
	if err := os.WriteFile(archivePath, []byte("archive-bytes"), 0o600); err != nil {
		t.Fatalf("expected archive fixture written, got %v", err)
	}

	// When
	_, err := resolveDiskImagePath(archivePath)
	// Then
	if err == nil {
		t.Fatal("expected error when no .img inside archive")
	}
}
