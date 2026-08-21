// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/assets/resources"
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

func TestDataFileName_ReplacesHyphensWithUnderscores(t *testing.T) {
	t.Parallel()

	got := dataFileName("infrastructure-presets", "linux", "amd64")
	want := "infrastructure_presets_linux_amd64.bin"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestGoIdentifier_CamelCasesAKebabCaseResourceID(t *testing.T) {
	t.Parallel()

	got := goIdentifier("infrastructure-presets")
	want := "infrastructurePresets"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestGeneratePlatform_WritesRealDataForDeclaredPlatform(t *testing.T) {
	t.Parallel()

	// Given
	content := []byte("runner binary bytes")
	server := newFixtureServer(t, "artifact.bin", content)
	def := runtimeartifacts.ResourceDefinition{
		Embed: runtimeartifacts.EmbedDefault,
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
	wantRegister := fmt.Sprintf(
		`runtimeartifacts.Register("embed-gen-test", embedGenTestData, %q)`, sha256Hex(content),
	)
	if !strings.Contains(string(goSource), wantRegister) {
		t.Fatalf("expected Register call with resource ID and content hash, got:\n%s", goSource)
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
		Embed: runtimeartifacts.EmbedDefault,
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
		Embed: runtimeartifacts.EmbedDefault,
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

func TestGeneratePlatform_SkipEmbedStillEmbedsWhenAlways(t *testing.T) {
	t.Parallel()

	// Given a resource declaring embed: always, e.g. the small, locally
	// sourced preset directories that unit tests must resolve by name even
	// under SKIP_EMBED.
	content := []byte("preset directory bytes")
	server := newFixtureServer(t, "artifact.bin", content)
	def := runtimeartifacts.ResourceDefinition{
		Embed: runtimeartifacts.EmbedAlways,
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
	g := &generator{manager: manager, outputDir: outputDir, goos: "darwin", goarch: "arm64", skipEmbed: true}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	goSource, err := os.ReadFile(filepath.Join(outputDir, "resources_darwin_arm64.go"))
	if err != nil {
		t.Fatalf("expected generated .go file, got %v", err)
	}
	if !strings.Contains(string(goSource), "go:embed") {
		t.Fatalf("expected embed: always to embed real data despite skipEmbed, got:\n%s", goSource)
	}
}

func TestGeneratePlatform_DoesNotTouchOtherPlatformsFile(t *testing.T) {
	t.Parallel()

	// Given
	content := []byte("runner binary bytes")
	server := newFixtureServer(t, "artifact.bin", content)
	def := runtimeartifacts.ResourceDefinition{
		Embed: runtimeartifacts.EmbedDefault,
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
			Embed: runtimeartifacts.EmbedDefault,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"darwin/arm64": {URL: server.URL + "/first.bin", Sha256: sha256Hex(firstContent)},
			},
		},
		"embed-gen-second": {
			Embed: runtimeartifacts.EmbedDefault,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"darwin/arm64": {URL: secondServer.URL + "/second.bin", Sha256: sha256Hex(secondContent)},
			},
		},
		"embed-gen-not-declared": {
			Embed: runtimeartifacts.EmbedDefault,
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
	wantFirst := fmt.Sprintf(
		`runtimeartifacts.Register("embed-gen-first", embedGenFirstData, %q)`, sha256Hex(firstContent),
	)
	if !strings.Contains(source, wantFirst) {
		t.Fatalf("expected the first resource to be registered, got:\n%s", source)
	}
	wantSecond := fmt.Sprintf(
		`runtimeartifacts.Register("embed-gen-second", embedGenSecondData, %q)`, sha256Hex(secondContent),
	)
	if !strings.Contains(source, wantSecond) {
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

// zipContaining builds an in-memory zip archive holding a single named entry,
// matching the shape of the real embedded resources: an archive that declares
// extract: true plus a resource_path pointing at a file inside it.
func zipContaining(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	buffer := bytes.Buffer{}
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("creating zip entry: %v", err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatalf("writing zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}

	return buffer.Bytes()
}

// Embedding stages the raw archive, so the generator neutralizes extract and
// the resource_path that goes with it. Leaving resource_path in place resolves
// it against the downloaded file instead of an extracted directory, yielding a
// path such as artifact.zip/launcher that cannot exist.
func TestGeneratePlatform_EmbedsRawArchiveWhenResourcePathIsDeclared(t *testing.T) {
	t.Parallel()

	// Given
	inner := []byte("runner binary bytes")
	archive := zipContaining(t, "launcher", inner)
	server := newFixtureServer(t, "artifact.zip", archive)
	def := runtimeartifacts.ResourceDefinition{
		Extract: true,
		Embed:   runtimeartifacts.EmbedDefault,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"darwin/arm64": {
				URL:          server.URL + "/artifact.zip",
				Sha256:       sha256Hex(archive),
				ResourcePath: "launcher",
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"embed-extract-test": def}
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(
		spec, t.TempDir(), "darwin", "arm64",
	)
	g := &generator{manager: manager, outputDir: outputDir, goos: "darwin", goarch: "arm64"}

	// When
	err := g.generatePlatform(context.Background(), spec)

	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	dataPath := filepath.Join(outputDir, "embed_extract_test_darwin_arm64.bin")
	dataBytes, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("expected embedded data file, got %v", err)
	}
	if !bytes.Equal(dataBytes, archive) {
		t.Fatalf("expected the raw archive bytes, got %d bytes", len(dataBytes))
	}

	// The caller's definition must survive untouched: the generator clears
	// resource_path on its own copy, not on the shared spec.
	if got := spec["embed-extract-test"].Artifact["darwin/arm64"].ResourcePath; got != "launcher" {
		t.Errorf("expected the spec's resource_path to be left alone, got %q", got)
	}
}
//nolint:paralleltest // changes the process's working directory.
func TestResolveRelativeLocalArtifacts_GenerationIsIdenticalFromAnyWorkingDirectory(t *testing.T) {
	// Given: a resource whose artifact is a relative bare local path.
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "assets", "fixture")
	if err := os.MkdirAll(fixtureDir, dirPerm); err != nil {
		t.Fatalf("failed to create fixture directory: %v", err)
	}
	fixturePath := filepath.Join(fixtureDir, "artifact.bin")
	if err := os.WriteFile(fixturePath, []byte("fixture content"), filePerm); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	spec := runtimeartifacts.ResourceSpec{
		"fixture-resource": {
			Embed: runtimeartifacts.EmbedDefault,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"linux/amd64": {URL: "assets/fixture/artifact.bin"},
			},
		},
	}

	generateFrom := func(cwd string) []byte {
		t.Helper()

		original, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to resolve current working directory: %v", err)
		}
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("failed to change to fixture working directory: %v", err)
		}
		defer func() {
			if err := os.Chdir(original); err != nil {
				t.Fatalf("failed to restore working directory: %v", err)
			}
		}()

		resolved := resolveRelativeLocalArtifacts(root, spec)
		outputDir := t.TempDir()
		manager := runtimeartifacts.NewResourceManagerForPlatform(resolved, t.TempDir(), "linux", "amd64")
		g := &generator{manager: manager, outputDir: outputDir, goos: "linux", goarch: "amd64"}
		if err := g.generatePlatform(context.Background(), resolved); err != nil {
			t.Fatalf("expected no error generating from %q, got %v", cwd, err)
		}
		data, err := os.ReadFile(filepath.Join(outputDir, "resources_linux_amd64.go"))
		if err != nil {
			t.Fatalf("expected generated file, got %v", err)
		}

		return data
	}

	// When
	first := generateFrom(t.TempDir())
	second := generateFrom(t.TempDir())

	// Then
	if string(first) != string(second) {
		t.Fatalf(
			"expected identical generated output regardless of working directory, got:\n%s\nvs\n%s",
			first, second,
		)
	}
}

//nolint:paralleltest // registers process-global embedded data via runtimeartifacts.Register.
func TestGeneratePlatform_EmbedsADirectoryAsAnExtractableArchive(t *testing.T) {
	// Given: an extract:true directory resource, matching how a built-in
	// preset directory is declared in resources.yaml.
	fixtureDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixtureDir, "main.tf"), []byte("main"), filePerm); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(fixtureDir, "files"), dirPerm); err != nil {
		t.Fatalf("failed to create nested fixture directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "files", "config"), []byte("config"), filePerm); err != nil {
		t.Fatalf("failed to write nested fixture file: %v", err)
	}
	def := runtimeartifacts.ResourceDefinition{
		Extract: true,
		Embed:   runtimeartifacts.EmbedDefault,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"any": {URL: "file://" + fixtureDir, DownloadPath: "dir-embed-test.tar.gz"},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"dir-embed-test": def}
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "linux", goarch: "amd64"}

	// When: the generator embeds the directory.
	err := g.generatePlatform(context.Background(), spec)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "dir_embed_test_linux_amd64.bin"))
	if err != nil {
		t.Fatalf("expected embedded data file, got %v", err)
	}

	// Then: registering those bytes and resolving through a fresh manager
	// (as the real running binary would) reproduces the original directory.
	runtimeartifacts.Register("dir-embed-test", data, sha256Hex(data))
	runtimeManager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	path, err := runtimeManager.Get(context.Background(), def, "dir-embed-test")
	if err != nil {
		t.Fatalf("expected the embedded archive to extract, got %v", err)
	}
	mainContent, err := os.ReadFile(filepath.Join(path, "main.tf"))
	if err != nil || string(mainContent) != "main" {
		t.Fatalf("expected extracted main.tf content, got %q, err %v", mainContent, err)
	}
	configContent, err := os.ReadFile(filepath.Join(path, "files", "config"))
	if err != nil || string(configContent) != "config" {
		t.Fatalf(
			"expected extracted nested files/config content, got %q, err %v", configContent, err,
		)
	}
}

//nolint:paralleltest // registers process-global embedded data via runtimeartifacts.Register.
func TestResolveResourceEmbed_GlobEmbedsMatchesNestedBelowRoot(t *testing.T) {
	// Given: a pattern nested below the resolved root's own top level, as
	// opposed to a bare "*" matching root's immediate children — the match
	// name ("aws") differs from the directory WalkDir must descend through
	// to reach it ("modules").
	fixtureDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fixtureDir, "modules", "aws"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(fixtureDir, "modules", "aws", "main.tf"), []byte("aws module"), filePerm,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	def := runtimeartifacts.ResourceDefinition{
		Glob:  true,
		Embed: runtimeartifacts.EmbedAlways,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"any": {
				URL:          "file://" + fixtureDir,
				ResourcePath: "modules/*",
				DownloadPath: "nested-glob-test.tar.gz",
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"nested-glob-test": def}
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "linux", goarch: "amd64"}

	// When
	embed, err := g.resolveResourceEmbed(context.Background(), "nested-glob-test", def)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if embed == nil || !slices.Contains(embed.Members, "aws") {
		t.Fatalf("expected member %q, got %v", "aws", embed)
	}

	// And: registering those bytes and resolving through a fresh manager (as
	// the real running binary would) reproduces the matched directory's own
	// content — a regression here previously embedded an empty archive,
	// since "modules" itself never matched the "aws" member filter.
	data, err := os.ReadFile(filepath.Join(outputDir, embed.DataFile))
	if err != nil {
		t.Fatalf("failed to read staged embed data: %v", err)
	}
	runtimeartifacts.Register("nested-glob-test", data, sha256Hex(data))
	runtimeManager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	path, err := runtimeManager.RequestMember(context.Background(), "nested-glob-test", "aws")
	if err != nil {
		t.Fatalf("expected the embedded archive to resolve member %q, got %v", "aws", err)
	}
	content, err := os.ReadFile(filepath.Join(path, "main.tf"))
	if err != nil || string(content) != "aws module" {
		t.Fatalf("expected extracted main.tf content, got %q, err %v", content, err)
	}
}

//nolint:paralleltest // registers process-global embedded data via runtimeartifacts.Register.
func TestResolveResourceEmbed_GlobArchivesSymlinksInsteadOfFailing(t *testing.T) {
	// Given: a matched directory containing a relative symlink, the way a
	// cloned git repository's own modules commonly do.
	fixtureDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fixtureDir, "aws"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(fixtureDir, "aws", "main.tf"), []byte("aws module"), filePerm,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	if err := os.Symlink("main.tf", filepath.Join(fixtureDir, "aws", "link.tf")); err != nil {
		t.Fatalf("failed to create fixture symlink: %v", err)
	}
	def := runtimeartifacts.ResourceDefinition{
		Glob:  true,
		Embed: runtimeartifacts.EmbedAlways,
		Artifact: map[string]runtimeartifacts.ArtifactSpec{
			"any": {
				URL:          "file://" + fixtureDir,
				ResourcePath: "*",
				DownloadPath: "symlink-glob-test.tar.gz",
			},
		},
	}
	spec := runtimeartifacts.ResourceSpec{"symlink-glob-test": def}
	outputDir := t.TempDir()
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	g := &generator{manager: manager, outputDir: outputDir, goos: "linux", goarch: "amd64"}

	// When
	embed, err := g.resolveResourceEmbed(context.Background(), "symlink-glob-test", def)
	// Then: generation succeeds instead of failing with "write too long".
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// And: the running binary materializes the symlink itself, not a
	// truncated copy of its target.
	data, err := os.ReadFile(filepath.Join(outputDir, embed.DataFile))
	if err != nil {
		t.Fatalf("failed to read staged embed data: %v", err)
	}
	runtimeartifacts.Register("symlink-glob-test", data, sha256Hex(data))
	runtimeManager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	path, err := runtimeManager.RequestMember(context.Background(), "symlink-glob-test", "aws")
	if err != nil {
		t.Fatalf("expected the embedded archive to resolve member %q, got %v", "aws", err)
	}
	linkTarget, err := os.Readlink(filepath.Join(path, "link.tf"))
	if err != nil || linkTarget != "main.tf" {
		t.Fatalf("expected link.tf to be a symlink to main.tf, got %q, err %v", linkTarget, err)
	}
}

func TestResolveResourceEmbed_RealPresetDirectoriesGlobToDeclaredMembers(t *testing.T) {
	t.Parallel()

	// Given: the real repository's own resources.yaml and directory tree.
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("failed to resolve repository root: %v", err)
	}
	spec, err := runtimeartifacts.ParseSpec(resources.ResourcesYAML)
	if err != nil {
		t.Fatalf("failed to parse resources.yaml: %v", err)
	}
	spec = resolveRelativeLocalArtifacts(root, spec)
	manager := runtimeartifacts.NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")
	g := &generator{manager: manager, outputDir: t.TempDir(), goos: "linux", goarch: "amd64"}

	// When
	infra, err := g.resolveResourceEmbed(context.Background(), "infrastructure-presets", spec["infrastructure-presets"])
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if infra == nil {
		t.Fatal("expected infrastructure-presets to embed for linux/amd64")
	}
	for _, want := range []string{"aws", "azure", "exoscale", "stackit", "local"} {
		if !slices.Contains(infra.Members, want) {
			t.Fatalf("expected member %q, got %v", want, infra.Members)
		}
	}

	install, err := g.resolveResourceEmbed(context.Background(), "installation-presets", spec["installation-presets"])
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if install == nil {
		t.Fatal("expected installation-presets to embed for linux/amd64")
	}
	for _, want := range []string{"ubuntu", "local"} {
		if !slices.Contains(install.Members, want) {
			t.Fatalf("expected member %q, got %v", want, install.Members)
		}
	}
}
