// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownloadRunner_DownloadsVerifiesAndExtractsLauncher(t *testing.T) {
	t.Parallel()

	// Given
	archive := zipFixture(t, map[string]string{"launcher": "runner"})
	_, baseURL := releaseFixtureDir(t, archive, true)
	downloadDir := t.TempDir()
	targetPath := filepath.Join(downloadDir, "mac-runner-aarch64")
	config := testRunnerConfig(downloadDir, baseURL)

	// When
	err := downloadRunner(context.Background(), config, targetPath)

	// Then
	if err != nil {
		t.Fatalf("expected download to succeed, got %v", err)
	}
	assertFileContent(t, targetPath, "runner")
	assertFileContent(t, versionFilePath(downloadDir), cacheKey(config)+"\n")
	assertExecutable(t, targetPath)
}

func TestDownloadRunner_RejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	// Given
	archive := zipFixture(t, map[string]string{"launcher": "runner"})
	releaseDir, baseURL := releaseFixtureDir(t, archive, false)
	checksumPath := filepath.Join(releaseDir, defaultAsset+".sha256")
	if err := os.WriteFile(checksumPath, []byte(strings.Repeat("0", sha256.Size*2)), filePerm); err != nil {
		t.Fatalf("failed to write checksum fixture: %v", err)
	}
	downloadDir := t.TempDir()
	config := testRunnerConfig(downloadDir, baseURL)

	// When
	err := downloadRunner(context.Background(), config, filepath.Join(downloadDir, "runner"))

	// Then
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestDownloadRunner_ReusesCachedRunner(t *testing.T) {
	t.Parallel()

	// Given
	downloadDir := t.TempDir()
	targetPath := filepath.Join(downloadDir, "mac-runner-aarch64")
	config := testRunnerConfig(downloadDir, "http://127.0.0.1:1")
	if err := os.WriteFile(targetPath, []byte("cached"), executablePerm); err != nil {
		t.Fatalf("failed to write cached runner: %v", err)
	}
	if err := os.WriteFile(versionFilePath(downloadDir), []byte(cacheKey(config)+"\n"), filePerm); err != nil {
		t.Fatalf("failed to write cache marker: %v", err)
	}

	// When
	err := downloadRunner(context.Background(), config, targetPath)

	// Then
	if err != nil {
		t.Fatalf("expected cached runner to be reused, got %v", err)
	}
	assertFileContent(t, targetPath, "cached")
}

func TestRunStage_StagesExplicitRunnerPath(t *testing.T) {
	t.Parallel()

	// Given
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "launcher")
	if err := os.WriteFile(sourcePath, []byte("runner"), executablePerm); err != nil {
		t.Fatalf("failed to write source runner: %v", err)
	}
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")

	// When
	err := runStage(context.Background(), []string{
		"-goos", "darwin",
		"-goarch", "arm64",
		"-runner-path", sourcePath,
		"-target", targetPath,
	})

	// Then
	if err != nil {
		t.Fatalf("expected stage to succeed, got %v", err)
	}
	assertFileContent(t, targetPath, "runner")
	assertExecutable(t, targetPath)
}

func TestRunPlaceholder_DoesNotOverwriteExistingTarget(t *testing.T) {
	t.Parallel()

	// Given
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")
	if err := os.WriteFile(targetPath, []byte("existing"), executablePerm); err != nil {
		t.Fatalf("failed to write existing placeholder: %v", err)
	}

	// When
	err := runPlaceholder([]string{"-target", targetPath})

	// Then
	if err != nil {
		t.Fatalf("expected placeholder to succeed, got %v", err)
	}
	assertFileContent(t, targetPath, "existing")
}

func testRunnerConfig(downloadDir, baseURL string) runnerConfig {
	return runnerConfig{
		repo:         "example/repo",
		version:      "v1",
		asset:        defaultAsset,
		resourcePath: defaultResourcePath,
		downloadDir:  downloadDir,
		baseURL:      baseURL,
	}
}

func releaseFixtureDir(t *testing.T, archive []byte, includeChecksum bool) (string, string) {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, defaultAsset), archive, filePerm); err != nil {
		t.Fatalf("failed to write archive fixture: %v", err)
	}
	if includeChecksum {
		sum := sha256.Sum256(archive)
		checksum := hex.EncodeToString(sum[:]) + "  " + defaultAsset + "\n"
		if err := os.WriteFile(filepath.Join(dir, defaultAsset+".sha256"), []byte(checksum), filePerm); err != nil {
			t.Fatalf("failed to write checksum fixture: %v", err)
		}
	}

	return dir, fileURLFromPath(t, dir)
}

func fileURLFromPath(t *testing.T, path string) string {
	t.Helper()

	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to resolve %s: %v", path, err)
	}
	slashPath := filepath.ToSlash(absPath)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}

	return (&url.URL{Scheme: "file", Path: slashPath}).String()
}

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close zip fixture: %v", err)
	}

	return buf.Bytes()
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if string(data) != expected {
		t.Fatalf("expected %s to contain %q, got %q", path, expected, string(data))
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("expected %s to be executable, got mode %v", path, info.Mode().Perm())
	}
}
