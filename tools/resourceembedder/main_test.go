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
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/exasol/exasol-personal/assets/resourcedata"
	"github.com/exasol/exasol-personal/internal/resource"
	"go.yaml.in/yaml/v3"
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

func zipFixtureBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	return buffer.Bytes()
}

// newGenerator builds a generator writing into a throwaway platform directory,
// with a cache of its own so runs cannot interfere with each other.
func newGenerator(t *testing.T, spec resource.ResourceSpec, skipEmbed bool) *generator {
	t.Helper()

	cacheRoot := t.TempDir()
	platformDir := filepath.Join(t.TempDir(), "linux_amd64")
	if err := os.MkdirAll(platformDir, dirPerm); err != nil {
		t.Fatalf("create platform dir: %v", err)
	}

	return &generator{
		manager: resource.NewResourceManagerForPlatform(
			spec, cacheRoot, "linux", "amd64",
		),
		platformDir: platformDir,
		goos:        "linux",
		goarch:      "amd64",
		skipEmbed:   skipEmbed,
	}
}

func generateSpec(t *testing.T, g *generator, spec resource.ResourceSpec) resource.ResourceSpec {
	t.Helper()

	if err := g.generate(context.Background(), spec); err != nil {
		t.Fatalf("generate: %v", err)
	}

	return readResolvedSpec(t, g)
}

func readResolvedSpec(t *testing.T, g *generator) resource.ResourceSpec {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(g.platformDir, resolvedSpecName))
	if err != nil {
		t.Fatalf("read resolved spec: %v", err)
	}
	var resolved resource.ResourceSpec
	if err := yaml.Unmarshal(raw, &resolved); err != nil {
		t.Fatalf("parse resolved spec: %v", err)
	}

	return resolved
}

func onlyArtifact(t *testing.T, def resource.ResourceDefinition) resource.ArtifactSpec {
	t.Helper()

	artifact, err := def.Resolve("linux", "amd64")
	if err != nil {
		t.Fatalf("resolve artifact: %v", err)
	}

	return artifact
}

func platformFiles(t *testing.T, g *generator) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(g.platformDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(g.platformDir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(rel))

		return nil
	})
	if err != nil {
		t.Fatalf("walk platform dir: %v", err)
	}
	slices.Sort(files)

	return files
}

func archiveSpec(t *testing.T, embed resource.EmbedMode) (resource.ResourceSpec, []byte) {
	t.Helper()

	payload := []byte("tool payload")
	server := newFixtureServer(t, "tool.tar.gz", payload)

	return resource.ResourceSpec{
		"tool": {
			Extract: true,
			Embed:   embed,
			Artifact: map[string]resource.ArtifactSpec{
				"any": {
					URL:          server.URL + "/tool.tar.gz",
					Sha256:       sha256Hex(payload),
					DownloadPath: "tool.tar.gz",
				},
			},
		},
	}, payload
}

// The resolved specification is what a build reads, so no directive that only
// the generator understands may survive into it.
func TestGenerate_ResolvedSpecCarriesNoBuildDirectives(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, false)
	generateSpec(t, g, spec)

	raw, err := os.ReadFile(filepath.Join(g.platformDir, resolvedSpecName))
	if err != nil {
		t.Fatalf("read resolved spec: %v", err)
	}
	for _, directive := range []string{"embed:", "glob:"} {
		if strings.Contains(string(raw), directive) {
			t.Fatalf("resolved spec still carries %q:\n%s", directive, raw)
		}
	}
}

func TestGenerate_EmbeddedResourcePointsAtEmbeddedData(t *testing.T) {
	t.Parallel()

	spec, payload := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, false)
	resolved := generateSpec(t, g, spec)

	artifact := onlyArtifact(t, resolved["tool"])
	if !strings.HasPrefix(artifact.URL, resource.EmbeddedURLScheme) {
		t.Fatalf("expected an embedded source, got %q", artifact.URL)
	}
	if artifact.Sha256 != sha256Hex(payload) {
		t.Fatalf("expected the blob's own hash, got %q", artifact.Sha256)
	}
	blob := filepath.Join(g.platformDir, filepath.FromSlash(
		strings.TrimPrefix(artifact.URL, resource.EmbeddedURLScheme),
	))
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("expected the blob to exist at %q, got %v", blob, err)
	}
}

func TestGenerate_UnembeddedResourcePointsUpstream(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedNever)
	g := newGenerator(t, spec, false)
	resolved := generateSpec(t, g, spec)

	artifact := onlyArtifact(t, resolved["tool"])
	if !strings.HasPrefix(artifact.URL, "http://") {
		t.Fatalf("expected the upstream source, got %q", artifact.URL)
	}
	if got := platformFiles(t, g); !slices.Equal(got, []string{resolvedSpecName}) {
		t.Fatalf("expected no blobs, got %v", got)
	}
}

// A blob's name has to say what it holds, since the extractor a build picks is
// chosen by that name.
func TestGenerate_BlobKeepsItsSourceExtension(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, false)
	resolved := generateSpec(t, g, spec)

	artifact := onlyArtifact(t, resolved["tool"])
	if !strings.HasSuffix(artifact.URL, ".tar.gz") {
		t.Fatalf("expected the blob to keep its extension, got %q", artifact.URL)
	}
}

// A relative local location cannot be resolved by a built binary, which has an
// arbitrary working directory and no repository root.
func TestGenerate_RejectsRelativeLocalLocationThatIsNotEmbedded(t *testing.T) {
	t.Parallel()

	spec := resource.ResourceSpec{
		"preset": {
			Artifact: map[string]resource.ArtifactSpec{"any": {URL: "assets/infrastructure"}},
		},
	}
	g := newGenerator(t, spec, false)

	err := g.generate(context.Background(), spec)
	if err == nil {
		t.Fatal("expected generation to fail")
	}
	if !strings.Contains(err.Error(), "preset") {
		t.Fatalf("expected the failing resource to be named, got %v", err)
	}
}

func TestGenerate_NoDataForUndeclaredPlatform(t *testing.T) {
	t.Parallel()

	spec := resource.ResourceSpec{
		"mac-only": {
			Embed: resource.EmbedAlways,
			Artifact: map[string]resource.ArtifactSpec{
				"darwin/arm64": {URL: "https://example.com/mac.tar.gz", Sha256: strings.Repeat("a", 64)},
			},
		},
	}
	g := newGenerator(t, spec, false)
	resolved := generateSpec(t, g, spec)

	if _, ok := resolved["mac-only"]; ok {
		t.Fatal("expected no entry for a platform the resource does not declare")
	}
	if got := platformFiles(t, g); !slices.Equal(got, []string{resolvedSpecName}) {
		t.Fatalf("expected no blobs, got %v", got)
	}
}

// Placeholder-only generation exists for build speed, so it must embed nothing
// while still leaving every resource resolvable.
func TestGenerate_SkipEmbedPointsUpstreamInsteadOfEmbedding(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedDefault)
	g := newGenerator(t, spec, true)
	resolved := generateSpec(t, g, spec)

	artifact := onlyArtifact(t, resolved["tool"])
	if !strings.HasPrefix(artifact.URL, "http://") {
		t.Fatalf("expected the upstream source, got %q", artifact.URL)
	}
	if got := platformFiles(t, g); !slices.Equal(got, []string{resolvedSpecName}) {
		t.Fatalf("expected nothing embedded, got %v", got)
	}
}

func TestGenerate_SkipEmbedStillEmbedsWhenAlways(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, true)
	resolved := generateSpec(t, g, spec)

	artifact := onlyArtifact(t, resolved["tool"])
	if !strings.HasPrefix(artifact.URL, resource.EmbeddedURLScheme) {
		t.Fatalf("expected an embedded source, got %q", artifact.URL)
	}
}

// The wrapper embeds the whole platform directory, so anything an earlier run
// left behind would ship rather than merely be ignored.
func TestGenerate_PrunesDataForAResourceNoLongerDeclared(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, false)
	generateSpec(t, g, spec)

	if len(platformFiles(t, g)) != 2 {
		t.Fatalf("expected a blob and a spec, got %v", platformFiles(t, g))
	}

	generateSpec(t, g, resource.ResourceSpec{})

	if got := platformFiles(t, g); !slices.Equal(got, []string{resolvedSpecName}) {
		t.Fatalf("expected the stale blob to be pruned, got %v", got)
	}
}

func TestGenerate_PrunesDataSkippedByPlaceholderOnlyMode(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedDefault)
	g := newGenerator(t, spec, false)
	generateSpec(t, g, spec)

	g.skipEmbed = true
	generateSpec(t, g, spec)

	if got := platformFiles(t, g); !slices.Equal(got, []string{resolvedSpecName}) {
		t.Fatalf("expected the skipped resource's data to be pruned, got %v", got)
	}
}

// Pruning must not reach outside the platform being generated.
func TestGenerate_PruningIsConfinedToTheTargetPlatform(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, false)

	other := filepath.Join(filepath.Dir(g.platformDir), "darwin_arm64")
	if err := os.MkdirAll(other, dirPerm); err != nil {
		t.Fatalf("create other platform dir: %v", err)
	}
	keep := filepath.Join(other, "resolved.yaml")
	if err := os.WriteFile(keep, []byte("other: {}\n"), filePerm); err != nil {
		t.Fatalf("write other platform spec: %v", err)
	}

	generateSpec(t, g, spec)

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("expected the other platform's data to survive, got %v", err)
	}
}

// A tracked .gitignore keeps the directory alive through a clone and keeps the
// embed compiling, so pruning must leave it be.
func TestGenerate_KeepsTheTrackedGitignore(t *testing.T) {
	t.Parallel()

	spec, _ := archiveSpec(t, resource.EmbedAlways)
	g := newGenerator(t, spec, false)
	ignore := filepath.Join(g.platformDir, gitignoreName)
	if err := os.WriteFile(ignore, []byte("*\n!.gitignore\n"), filePerm); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}

	generateSpec(t, g, spec)

	if _, err := os.Stat(ignore); err != nil {
		t.Fatalf("expected .gitignore to survive pruning, got %v", err)
	}
}

func globSpec(t *testing.T, pattern string) (resource.ResourceSpec, string) {
	t.Helper()

	srcDir := t.TempDir()
	for _, name := range []string{"aws", "azure"} {
		if err := os.MkdirAll(filepath.Join(srcDir, name), dirPerm); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if err := os.WriteFile(
			filepath.Join(srcDir, name, "main.tf"), []byte(name), filePerm,
		); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("x"), filePerm); err != nil {
		t.Fatalf("write README: %v", err)
	}

	return resource.ResourceSpec{
		"presets": {
			Embed:    resource.EmbedAlways,
			Glob:     pattern,
			Artifact: map[string]resource.ArtifactSpec{"any": {URL: "file://" + srcDir}},
		},
	}, srcDir
}

// A glob is matched once, at build time, and each match becomes a resource in
// its own right; the group itself never reaches a build.
func TestGenerate_GlobExpandsIntoOneResourcePerMatch(t *testing.T) {
	t.Parallel()

	spec, _ := globSpec(t, "*")
	g := newGenerator(t, spec, false)
	resolved := generateSpec(t, g, spec)

	for _, want := range []string{"presets/aws", "presets/azure", "presets/README.md"} {
		if _, ok := resolved[want]; !ok {
			t.Fatalf("expected %q among %v", want, slices.Sorted(maps(resolved)))
		}
	}
	if _, ok := resolved["presets"]; ok {
		t.Fatal("expected the group itself not to reach the resolved spec")
	}
}

// A pattern may match a file as readily as a directory.
func TestGenerate_GlobMatchesFilesAndDirectoriesAlike(t *testing.T) {
	t.Parallel()

	// Given
	spec, _ := globSpec(t, "*")
	g := newGenerator(t, spec, false)

	// When
	resolved := generateSpec(t, g, spec)

	// Then
	dirArtifact := onlyArtifact(t, resolved["presets/aws"])
	if !strings.HasSuffix(dirArtifact.URL, ".tar.gz") {
		t.Fatalf("expected a matched directory to be archived, got %q", dirArtifact.URL)
	}
	if !resolved["presets/aws"].Extract {
		t.Fatal("expected a matched directory to be extracted")
	}
	fileArtifact := onlyArtifact(t, resolved["presets/README.md"])
	if !strings.HasSuffix(fileArtifact.URL, ".md") {
		t.Fatalf("expected a matched file to keep its extension, got %q", fileArtifact.URL)
	}
	if resolved["presets/README.md"].Extract {
		t.Fatal("expected a matched file to resolve without extraction")
	}

	blobPath := strings.TrimPrefix(fileArtifact.URL, resource.EmbeddedURLScheme)
	blob, err := os.ReadFile(filepath.Join(g.platformDir, filepath.FromSlash(blobPath)))
	if err != nil {
		t.Fatalf("read embedded file blob: %v", err)
	}
	rawSpec, err := yaml.Marshal(resolved)
	if err != nil {
		t.Fatalf("marshal runtime spec: %v", err)
	}
	runtimeManager, err := resource.New(resource.Options{
		Spec:      rawSpec,
		Blobs:     fstest.MapFS{blobPath: {Data: blob}},
		CacheRoot: t.TempDir(),
		Platform:  resource.Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err != nil {
		t.Fatalf("create runtime resolver: %v", err)
	}
	resolvedPath, err := runtimeManager.Request(context.Background(), "presets/README.md")
	if err != nil {
		t.Fatalf("resolve embedded file member: %v", err)
	}
	if content, err := os.ReadFile(resolvedPath); err != nil || string(content) != "x" {
		t.Fatalf("embedded file content = %q, error = %v", content, err)
	}
}

func TestGenerate_UnembeddedNestedGlobPreservesFullMatchedSubpath(t *testing.T) {
	t.Parallel()

	// Given
	archive := zipFixtureBytes(t, map[string]string{
		"catalog/modules/aws/main.tf": "aws",
	})
	server := newFixtureServer(t, "catalog", archive)
	spec := resource.ResourceSpec{
		"presets": {
			Extract: true,
			Glob:    "modules/*",
			Artifact: map[string]resource.ArtifactSpec{
				"any": {
					URL:          server.URL + "/catalog",
					Sha256:       sha256Hex(archive),
					DownloadPath: "catalog.zip",
					Subpath:      "catalog",
				},
			},
		},
	}
	generator := newGenerator(t, spec, false)

	// When
	resolved := generateSpec(t, generator, spec)

	// Then
	artifact := onlyArtifact(t, resolved["presets/aws"])
	if artifact.Subpath != "catalog/modules/aws" {
		t.Fatalf("subpath = %q, want %q", artifact.Subpath, "catalog/modules/aws")
	}
	rawSpec, err := yaml.Marshal(resolved)
	if err != nil {
		t.Fatalf("marshal runtime spec: %v", err)
	}
	runtimeManager, err := resource.New(resource.Options{
		Spec:      rawSpec,
		CacheRoot: t.TempDir(),
		Platform:  resource.Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err != nil {
		t.Fatalf("create runtime resolver: %v", err)
	}
	resolvedPath, err := runtimeManager.Request(context.Background(), "presets/aws")
	if err != nil {
		t.Fatalf("resolve upstream member: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(resolvedPath, "main.tf"))
	if err != nil || string(content) != "aws" {
		t.Fatalf("upstream member content = %q, error = %v", content, err)
	}
}

func TestGenerate_GlobRejectsAnEmptyPattern(t *testing.T) {
	t.Parallel()

	spec, srcDir := globSpec(t, "")
	// An empty pattern is indistinguishable from no glob at all in the struct,
	// so drive the matcher directly.
	if _, err := globMatches(srcDir, ""); err == nil {
		t.Fatal("expected an empty pattern to be rejected")
	}
	_ = spec
}

// A clone's metadata directory would otherwise match a top-level pattern like
// any other entry.
func TestGlobMatches_ExcludesRepositoryMetadata(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	for _, name := range []string{".git", "aws"} {
		if err := os.MkdirAll(filepath.Join(srcDir, name), dirPerm); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	matches, err := globMatches(srcDir, "*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if _, ok := matches[".git"]; ok {
		t.Fatal("expected repository metadata to be excluded")
	}
	if _, ok := matches["aws"]; !ok {
		t.Fatal("expected the real match to survive")
	}
}

func TestGlobMatches_RejectsMatchesSharingAMemberName(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(srcDir, dir), dirPerm); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
		if err := os.WriteFile(
			filepath.Join(srcDir, dir, "aws"), []byte("x"), filePerm,
		); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	if _, err := globMatches(srcDir, "*/aws"); err == nil {
		t.Fatal("expected a duplicate member name to be rejected")
	}
}

// Identical content must produce an identical identity, because a generated
// blob's hash is what tells two builds' content apart.
func TestGenerate_IdenticalContentProducesIdenticalIdentity(t *testing.T) {
	t.Parallel()

	spec, _ := globSpec(t, "*")

	first := newGenerator(t, spec, false)
	second := newGenerator(t, spec, false)
	firstSpec := generateSpec(t, first, spec)
	secondSpec := generateSpec(t, second, spec)

	firstHash := onlyArtifact(t, firstSpec["presets/aws"]).Sha256
	secondHash := onlyArtifact(t, secondSpec["presets/aws"]).Sha256
	if firstHash != secondHash {
		t.Fatalf("identity %q then %q", firstHash, secondHash)
	}
}

// The real catalog must expand to the preset directories it ships, so adding
// one needs no source-code edit.
func TestGenerate_RealPresetDirectoriesExpandToTheirMembers(t *testing.T) {
	t.Parallel()

	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	spec, err := resource.ParseSpec(resourcedata.ResourcesYAML)
	if err != nil {
		t.Fatalf("parse authoring spec: %v", err)
	}
	spec = resolveRelativeLocalArtifacts(root, spec)

	g := newGenerator(t, spec, true)
	resolved := generateSpec(t, g, spec)

	entries, err := os.ReadDir(filepath.Join(root, "assets", "infrastructure"))
	if err != nil {
		t.Fatalf("read preset directories: %v", err)
	}
	for _, entry := range entries {
		want := "infrastructure-presets/" + entry.Name()
		if _, ok := resolved[want]; !ok {
			t.Fatalf("expected %q in the resolved specification", want)
		}
	}
}

func maps(spec resource.ResourceSpec) func(func(string) bool) {
	return func(yield func(string) bool) {
		for id := range spec {
			if !yield(id) {
				return
			}
		}
	}
}

// Generation must not depend on the working directory it is invoked from.
func TestResolveRelativeLocalArtifacts_IsIndependentOfWorkingDirectory(t *testing.T) {
	t.Parallel()

	spec := resource.ResourceSpec{
		"preset": {
			Artifact: map[string]resource.ArtifactSpec{"any": {URL: "assets/infrastructure"}},
		},
	}

	resolved := resolveRelativeLocalArtifacts("/repo", spec)

	got := resolved["preset"].Artifact["any"].URL
	if want := resource.FileURLScheme + filepath.Join("/repo", "assets/infrastructure"); got != want {
		t.Fatalf("resolved URL = %q, want %q", got, want)
	}
}
