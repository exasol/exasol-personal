// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type tarFixtureEntry struct {
	Content string
	Mode    int64
	IsDir   bool
}

func TestParseSpec_AllowsEmptySpec(t *testing.T) {
	t.Parallel()

	// When
	spec, err := ParseSpec([]byte("{}"))
	// Then
	if err != nil {
		t.Fatalf("expected empty spec to be valid, got %v", err)
	}
	if len(spec) != 0 {
		t.Fatalf("expected empty spec, got %d resources", len(spec))
	}
}

func TestParseSpec_RejectsResourcePathWithoutExtraction(t *testing.T) {
	t.Parallel()

	// Given
	raw := []byte(`
artifact:
  extract: false
  artifact:
    linux/amd64:
      url: https://example.com/artifact-linux.tar.gz
      sha256: deadbeef
      resource_path: tofu
`)

	// When
	_, err := ParseSpec(raw)
	// Then
	if err == nil ||
		!strings.Contains(err.Error(), "must not define resource_path without archive extraction") {
		t.Fatalf("expected resource_path validation error, got %v", err)
	}
}

func TestManager_RequestUsesDownloadPathFallback(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	artifactPath := writeTarGzFixture(t, deploymentDir, "artifact-linux-amd64.tar.gz", "tool")
	artifactData, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	server := newArtifactServer(t, "", artifactData)
	spec := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/",
					Sha256:       sha256OfTestFile(t, artifactPath),
					DownloadPath: "artifact.tar.gz",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")

	// When
	path, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expectedPath := filepath.Join(deploymentDir, "resources", "artifact", "artifact.tar.gz")
	if path != expectedPath {
		t.Fatalf("expected artifact path %q, got %q", expectedPath, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected artifact path to exist, got %v", err)
	}
}

func TestManager_RequestRejectsDownloadPathEscape(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	artifactPath := writeTarGzFixture(t, deploymentDir, "artifact-linux-amd64.tar.gz", "tool")
	artifactData, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	server := newArtifactServer(t, "", artifactData)
	spec := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/",
					Sha256:       sha256OfTestFile(t, artifactPath),
					DownloadPath: "../escape.tar.gz",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")

	// When
	_, err = manager.Request(context.Background(), "artifact")
	// Then
	if err == nil || !strings.Contains(err.Error(), "must stay within") {
		t.Fatalf("expected resource-dir containment error, got %v", err)
	}
}

func TestManager_RequestUsesPlatformVariantAndCachesIt(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(
		t,
		deploymentDir,
		"artifact-linux-amd64.tgz",
		map[string]tarFixtureEntry{
			"tool": {
				Content: "tool",
				Mode:    0o640,
			},
			"nested/README": {
				Content: "readme",
				Mode:    0o600,
			},
			"nested/config/": {
				Mode:  0o750,
				IsDir: true,
			},
			"nested/config/x": {
				Content: "x",
				Mode:    0o644,
			},
		},
	)
	sum := sha256OfTestFile(t, archivePath)
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	server := newArtifactServer(t, "artifact.tgz", archiveData)
	spec := ResourceSpec{
		"artifact": {
			Extract: true,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.tgz",
					Sha256:       sum,
					ResourcePath: "tool",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")

	// When
	path1, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path1 == "" {
		t.Fatal("expected resolved path")
	}
	expectedPath := filepath.Join(deploymentDir, "resources", "artifact", "artifact", "tool")
	if path1 != expectedPath {
		t.Fatalf("expected extracted file path %q, got %q", expectedPath, path1)
	}
	if _, err := os.Stat(path1); err != nil {
		t.Fatalf("expected resolved path to exist, got %v", err)
	}
	readmePath := filepath.Join(
		deploymentDir,
		"resources",
		"artifact",
		"artifact",
		"nested",
		"README",
	)
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("expected extracted nested README to exist, got %v", err)
	}
	configPath := filepath.Join(
		deploymentDir,
		"resources",
		"artifact",
		"artifact",
		"nested",
		"config",
		"x",
	)
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected extracted nested config file to exist, got %v", err)
	}
	toolInfo, err := os.Stat(path1)
	if err != nil {
		t.Fatalf("expected extracted tool stats, got %v", err)
	}
	if toolInfo.Mode().Perm() != 0o640 {
		t.Fatalf("expected extracted tool mode 0640, got %v", toolInfo.Mode().Perm())
	}

	// When
	path2, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error on cache hit, got %v", err)
	}
	if path2 != path1 {
		t.Fatalf("expected cache hit to return same path, got %q and %q", path1, path2)
	}
}

func TestManager_RequestReportsChecksumMismatch(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	archivePath := writeTarGzFixture(t, deploymentDir, "artifact-linux-amd64.tgz", "tool")
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	server := newArtifactServer(t, "artifact.tgz", archiveData)
	spec := ResourceSpec{
		"artifact": {
			Extract: true,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.tgz",
					Sha256:       strings.Repeat("0", 64),
					ResourcePath: "tool",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")

	// When
	_, err = manager.Request(context.Background(), "artifact")
	// Then
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "got") {
		t.Fatalf("expected checksum details in error, got %v", err)
	}
}

func TestManager_RequestRefreshesWhenChecksumChanges(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	firstPath := writeTarGzFixture(t, deploymentDir, "artifact-linux-amd64.tgz", "tool-v1")
	firstData, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("failed to read first artifact fixture: %v", err)
	}
	secondPath := writeTarGzFixture(t, deploymentDir, "artifact-linux-amd64-v2.tgz", "tool-v2")
	secondData, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("failed to read second artifact fixture: %v", err)
	}
	artifactData := firstData
	server := newMutableArtifactServer(t, "artifact.tgz", &artifactData)
	specV1 := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.tgz",
					Sha256:       sha256OfTestFile(t, firstPath),
					DownloadPath: "artifact.tar.gz",
				},
			},
		},
	}
	managerV1 := NewResourceManagerForPlatform(specV1, deploymentDir, "linux", "amd64")

	// When
	path1, err := managerV1.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	if path1 == "" {
		t.Fatal("expected first resolved path")
	}

	artifactData = secondData
	specV2 := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.tgz",
					Sha256:       sha256OfTestFile(t, secondPath),
					DownloadPath: "artifact.tar.gz",
				},
			},
		},
	}
	managerV2 := NewResourceManagerForPlatform(specV2, deploymentDir, "linux", "amd64")

	// When
	path2, err := managerV2.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected checksum refresh to succeed, got %v", err)
	}
	if path2 != path1 {
		t.Fatalf("expected refresh to reuse the same cache path, got %q and %q", path1, path2)
	}
	data, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("expected refreshed artifact to be readable, got %v", err)
	}
	if string(data) != string(secondData) {
		t.Fatal("expected refreshed artifact content to change")
	}
}

func writeTarGzFixture(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	outputFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}
	gzipWriter := gzip.NewWriter(outputFile)
	tarWriter := tar.NewWriter(gzipWriter)

	if err := tarWriter.WriteHeader(&tar.Header{
		Name:     content,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write tar payload: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	if err := outputFile.Close(); err != nil {
		t.Fatalf("failed to close fixture file: %v", err)
	}

	return path
}

func writeTarGzMultiFixture(
	t *testing.T,
	dir, name string,
	entries map[string]tarFixtureEntry,
) string {
	t.Helper()

	path := filepath.Join(dir, name)
	outputFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create fixture: %v", err)
	}
	gzipWriter := gzip.NewWriter(outputFile)
	tarWriter := tar.NewWriter(gzipWriter)

	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, entryName := range keys {
		entry := entries[entryName]
		if entry.IsDir || strings.HasSuffix(entryName, "/") {
			if err := tarWriter.WriteHeader(&tar.Header{
				Name:     entryName,
				Mode:     entry.Mode,
				Typeflag: tar.TypeDir,
			}); err != nil {
				t.Fatalf("failed to write tar directory header: %v", err)
			}

			continue
		}

		if err := tarWriter.WriteHeader(&tar.Header{
			Name:     entryName,
			Mode:     entry.Mode,
			Size:     int64(len(entry.Content)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if _, err := tarWriter.Write([]byte(entry.Content)); err != nil {
			t.Fatalf("failed to write tar payload: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	if err := outputFile.Close(); err != nil {
		t.Fatalf("failed to close fixture file: %v", err)
	}

	return path
}

func sha256OfTestFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func newArtifactServer(t *testing.T, artifactName string, data []byte) *httptest.Server {
	t.Helper()

	handler := func(writer http.ResponseWriter, request *http.Request) {
		expectedPath := "/"
		if artifactName != "" {
			expectedPath = "/" + artifactName
		}
		if request.URL.Path != expectedPath {
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

func newMutableArtifactServer(t *testing.T, artifactName string, data *[]byte) *httptest.Server {
	t.Helper()

	handler := func(writer http.ResponseWriter, request *http.Request) {
		expectedPath := "/"
		if artifactName != "" {
			expectedPath = "/" + artifactName
		}
		if request.URL.Path != expectedPath {
			http.NotFound(writer, request)
			return
		}

		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(*data)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)

	return server
}
