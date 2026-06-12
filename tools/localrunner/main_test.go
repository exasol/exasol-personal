// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

func TestResolveRunnerSourceFromSpec_DownloadsAndExtractsLauncher(t *testing.T) {
	t.Parallel()

	// Given
	archive := zipFixture(t, map[string]string{"launcher": "runner"})
	server := artifactServer(t, "mac-runner-aarch64.zip", archive)
	config := runnerConfig{
		resourceID:   "runner",
		cacheRoot:    filepath.Join(t.TempDir(), "cache"),
		targetGOOS:   "darwin",
		targetGOARCH: "arm64",
	}
	spec := runtimeartifacts.ResourceSpec{
		"runner": {
			Extract: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"darwin/arm64": {
					URL:          server.URL + "/mac-runner-aarch64.zip",
					Sha256:       checksumString(archive),
					DownloadPath: "mac-runner-aarch64.zip",
					ResourcePath: "launcher",
				},
			},
		},
	}

	// When
	sourcePath, err := resolveRunnerSourceFromSpec(context.Background(), config, spec)
	// Then
	if err != nil {
		t.Fatalf("expected resource resolution to succeed, got %v", err)
	}
	assertFileContent(t, sourcePath, "runner")
	assertExecutable(t, sourcePath)
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

func TestRunStage_IsIdempotentForExistingTarget(t *testing.T) {
	t.Parallel()

	// Given
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "launcher")
	if err := os.WriteFile(sourcePath, []byte("runner"), executablePerm); err != nil {
		t.Fatalf("failed to write source runner: %v", err)
	}
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")
	if err := os.WriteFile(targetPath, []byte("runner"), 0o600); err != nil {
		t.Fatalf("failed to write target runner: %v", err)
	}

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

func TestRunStage_SkipsNonDarwinArm64Target(t *testing.T) {
	t.Parallel()

	// Given
	targetPath := filepath.Join(t.TempDir(), "mac-runner-aarch64")

	// When
	err := runStage(context.Background(), []string{
		"-goos", "linux",
		"-goarch", "amd64",
		"-target", targetPath,
	})
	// Then
	if err != nil {
		t.Fatalf("expected stage to skip unsupported target, got %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected target to be absent after skip, got %v", err)
	}
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

func artifactServer(t *testing.T, artifactName string, data []byte) *httptest.Server {
	t.Helper()

	handler := func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/"+artifactName {
			http.NotFound(writer, request)
			return
		}

		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(data)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)

	return server
}

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		header.SetMode(0o755)
		entry, err := writer.CreateHeader(header)
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

func checksumString(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
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

func TestParseRunnerFlags_UsesTargetEnvironment(t *testing.T) {
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOARCH", "arm64")

	// When
	config, err := parseRunnerFlags("stage", nil)
	// Then
	if err != nil {
		t.Fatalf("expected flags to parse, got %v", err)
	}
	if config.targetGOOS != "darwin" || config.targetGOARCH != "arm64" {
		t.Fatalf(
			"expected target from environment, got %s/%s",
			config.targetGOOS,
			config.targetGOARCH,
		)
	}
	if strings.TrimSpace(config.resourceID) != defaultResourceID {
		t.Fatalf("expected default resource ID %q, got %q", defaultResourceID, config.resourceID)
	}
}
