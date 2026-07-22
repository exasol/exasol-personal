// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

func newFixtureServer(t *testing.T, name string, data []byte) *httptest.Server {
	t.Helper()

	handler := func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/"+name {
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func TestGeneratePlatform_WritesRealDataForDeclaredPlatform(t *testing.T) {
	t.Parallel()

	// Given
	content := []byte("runner binary bytes")
	server := newFixtureServer(t, "artifact.bin", content)
	def := runtimeartifacts.ResourceDefinition{
		Embed: true,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"darwin/arm64": {
				URL:    server.URL + "/artifact.bin",
				Sha256: sha256Hex(content),
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"embed-gen-test": def}
	cacheDir := t.TempDir()
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, cacheDir, "darwin", "arm64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "darwin", goarch: "arm64"}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	goPath := filepath.Join(outputDir, "resources_darwin_arm64.go")
	dataPath := filepath.Join(outputDir, "embed_gen_test_darwin_arm64.bin")
	goSource, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("expected generated .go file, got %v", err)
	}
	if !strings.Contains(string(goSource), "//go:build darwin && arm64") {
		t.Fatalf("expected build tag for darwin/arm64, got:\n%s", goSource)
	}
	if !strings.Contains(string(goSource), `runtimeartifacts.Register("embed-gen-test", embedGenTestData)`) {
		t.Fatalf("expected Register call with resource ID, got:\n%s", goSource)
	}
	dataBytes, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("expected embedded data file, got %v", err)
	}
	if string(dataBytes) != string(content) {
		t.Fatalf("expected raw, checksum-verified artifact bytes, got %q", dataBytes)
	}
}

func TestGeneratePlatform_WritesPlaceholderForUndeclaredPlatform(t *testing.T) {
	t.Parallel()

	// Given
	def := runtimeartifacts.ResourceDefinition{
		Embed: true,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"darwin/arm64": {
				URL:    "https://example.com/artifact.bin",
				Sha256: "deadbeef",
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"embed-gen-test": def}
	cacheDir := t.TempDir()
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, cacheDir, "linux", "amd64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "linux", goarch: "amd64"}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	goPath := filepath.Join(outputDir, "resources_linux_amd64.go")
	goSource, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("expected generated placeholder .go file, got %v", err)
	}
	if !strings.Contains(string(goSource), "//go:build linux && amd64") {
		t.Fatalf("expected build tag for linux/amd64, got:\n%s", goSource)
	}
	if strings.Contains(string(goSource), "go:embed") {
		t.Fatalf("expected placeholder to embed nothing, got:\n%s", goSource)
	}
	if !strings.Contains(string(goSource), "embed-gen-test: no embedded data for linux/amd64") {
		t.Fatalf("expected a comment explaining the skipped resource, got:\n%s", goSource)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "embed_gen_test_linux_amd64.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected no data file for an undeclared platform, got err=%v", err)
	}
}

func TestGeneratePlatform_SkipEmbedNeverFetchesEvenOnDeclaredPlatform(t *testing.T) {
	t.Parallel()

	// Given
	def := runtimeartifacts.ResourceDefinition{
		Embed: true,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"darwin/arm64": {
				// A URL that would fail loudly if ever dialed, proving
				// skipEmbed never attempts a network fetch even though this
				// platform is declared.
				URL:    "http://127.0.0.1:0/unreachable.bin",
				Sha256: "deadbeef",
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"embed-gen-test": def}
	cacheDir := t.TempDir()
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, cacheDir, "darwin", "arm64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "darwin", goarch: "arm64", skipEmbed: true}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	goSource, err := os.ReadFile(filepath.Join(outputDir, "resources_darwin_arm64.go"))
	if err != nil {
		t.Fatalf("expected generated placeholder .go file, got %v", err)
	}
	if strings.Contains(string(goSource), "go:embed") {
		t.Fatalf("expected skipEmbed to never embed data, got:\n%s", goSource)
	}
}

func TestGeneratePlatform_DoesNotTouchOtherPlatformsFile(t *testing.T) {
	t.Parallel()

	// Given
	content := []byte("runner binary bytes")
	server := newFixtureServer(t, "artifact.bin", content)
	def := runtimeartifacts.ResourceDefinition{
		Embed: true,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"darwin/arm64": {
				URL:    server.URL + "/artifact.bin",
				Sha256: sha256Hex(content),
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"embed-gen-test": def}
	cacheDir := t.TempDir()
	outputDir := t.TempDir()
	untouchedPath := filepath.Join(outputDir, "resources_linux_amd64.go")
	if err := os.WriteFile(untouchedPath, []byte("existing content"), filePerm); err != nil {
		t.Fatalf("failed to seed existing platform file: %v", err)
	}
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, cacheDir, "darwin", "arm64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "darwin", goarch: "arm64"}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	untouched, err := os.ReadFile(untouchedPath)
	if err != nil {
		t.Fatalf("expected untouched platform file to remain, got %v", err)
	}
	if string(untouched) != "existing content" {
		t.Fatalf("expected another platform's file to be left alone, got %q", untouched)
	}
}

func TestGeneratePlatform_CombinesMultipleResourcesIntoOneFile(t *testing.T) {
	t.Parallel()

	// Given
	firstContent := []byte("first resource bytes")
	secondContent := []byte("second resource bytes")
	server := newFixtureServer(t, "first.bin", firstContent)
	secondServer := newFixtureServer(t, "second.bin", secondContent)
	spec := runtimeartifacts.ResourceSpec{
		"embed-gen-first": {
			Embed: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"darwin/arm64": {URL: server.URL + "/first.bin", Sha256: sha256Hex(firstContent)},
			},
		},
		"embed-gen-second": {
			Embed: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"darwin/arm64": {URL: secondServer.URL + "/second.bin", Sha256: sha256Hex(secondContent)},
			},
		},
		"embed-gen-not-declared": {
			Embed: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"linux/amd64": {URL: "https://example.com/artifact.bin", Sha256: "deadbeef"},
			},
		},
	}
	cacheDir := t.TempDir()
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, cacheDir, "darwin", "arm64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "darwin", goarch: "arm64"}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("failed to read output dir: %v", err)
	}
	goFiles := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") {
			goFiles++
		}
	}
	if goFiles != 1 {
		t.Fatalf("expected exactly one generated .go file for the platform, got %d (%v)", goFiles, entries)
	}
	goSource, err := os.ReadFile(filepath.Join(outputDir, "resources_darwin_arm64.go"))
	if err != nil {
		t.Fatalf("expected a combined generated .go file, got %v", err)
	}
	source := string(goSource)
	if !strings.Contains(source, `runtimeartifacts.Register("embed-gen-first", embedGenFirstData)`) {
		t.Fatalf("expected the first resource to be registered, got:\n%s", source)
	}
	if !strings.Contains(source, `runtimeartifacts.Register("embed-gen-second", embedGenSecondData)`) {
		t.Fatalf("expected the second resource to be registered, got:\n%s", source)
	}
	if strings.Contains(source, "embed-gen-not-declared") == false {
		t.Fatalf("expected the undeclared resource to at least be mentioned in a comment, got:\n%s", source)
	}
	if strings.Contains(source, `runtimeartifacts.Register("embed-gen-not-declared"`) {
		t.Fatalf("expected the undeclared resource to not be embedded, got:\n%s", source)
	}
	for _, dataFile := range []string{"embed_gen_first_darwin_arm64.bin", "embed_gen_second_darwin_arm64.bin"} {
		if _, err := os.Stat(filepath.Join(outputDir, dataFile)); err != nil {
			t.Fatalf("expected data file %s to exist, got %v", dataFile, err)
		}
	}
}
