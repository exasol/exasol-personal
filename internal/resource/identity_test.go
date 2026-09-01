// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentIdentity_PrefersDeclaredChecksum(t *testing.T) {
	t.Parallel()

	const declared = "AbCdEf0123456789"
	got := contentIdentity(declared, "git-commit:deadbeef")
	if want := "sha256:abcdef0123456789"; got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestContentIdentity_FallsBackToProbe(t *testing.T) {
	t.Parallel()

	if got := contentIdentity("", "git-commit:deadbeef"); got != "git-commit:deadbeef" {
		t.Fatalf("identity = %q, want the probed value", got)
	}
}

func TestResolver_ReportsIdentityResolutionFailure(t *testing.T) {
	t.Parallel()

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file:///nonexistent/probe/path"},
		},
	}

	_, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected the probe failure to be reported, got %v", err)
	}
}

func TestResolver_ProbedIdentityAvoidsSecondTransfer(t *testing.T) {
	t.Parallel()

	var downloads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloads++
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	first, err := resolveTestDefinition(context.Background(), resolver, def, "payload")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := resolveTestDefinition(context.Background(), resolver, def, "payload")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if first != second {
		t.Fatalf("expected the same cached path, got %q then %q", first, second)
	}
	if downloads != 1 {
		t.Fatalf("expected one download, got %d", downloads)
	}
}

func TestResolver_RejectsAndDoesNotRecordMismatchedContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not what was declared"))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	if _, err := resolveTestDefinition(context.Background(), resolver, def, "payload"); err == nil {
		t.Fatal("expected a checksum mismatch error")
	}

	index, err := resolver.cache.readIndex()
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("expected no recorded cache entry, got %d", len(index.Entries))
	}
}

func TestResolver_LocalDirectoryOccupiesNoCacheEntry(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	manifest := filepath.Join(presetDir, "infrastructure.yaml")
	if err := os.WriteFile(manifest, nil, filePerm); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{anyPlatformKey: {URL: "file://" + presetDir}},
	}

	got, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != presetDir {
		t.Fatalf("expected the directory itself, got %q", got)
	}
}

func writeIndexFixture(t *testing.T, cache *Cache, body string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(cache.IndexPath()), dirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cache.IndexPath(), []byte(body), filePerm); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

func newIsolatedCache(t *testing.T) *Cache {
	t.Helper()

	cacheDir := t.TempDir()

	return NewCache(cacheDir, filepath.Join(cacheDir, cacheConfigFileName))
}

func TestResolver_StartsFreshOnPriorCacheSchemaVersion(t *testing.T) {
	t.Parallel()

	// Given
	cache := newIsolatedCache(t)
	content := []byte("current")
	server, downloads := newCountingArtifactServer(t, "tool.bin", content)
	identity := "sha256:" + sha256OfBytes(content)
	key := cacheKeyFor(identity, Locator{URL: server.URL + "/tool.bin"}, "tool.bin")
	entryDir := cache.entryDir(key)
	if err := os.MkdirAll(entryDir, dirPerm); err != nil {
		t.Fatalf("create old cache entry: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(entryDir, "tool.bin"),
		[]byte("old"),
		filePerm,
	); err != nil {
		t.Fatalf("write old cached artifact: %v", err)
	}
	writeIndexFixture(t, cache, `{"version":1,"entries":{"`+key+`":{`+
		`"resourceIds":["tool"],"identity":"`+identity+`",`+
		`"downloadPath":"tool.bin"}}}`)
	resolver := newTestResolverForPlatform(t, ResourceSpec{
		"tool": {Artifact: map[string]ArtifactSpec{anyPlatformKey: {
			URL:          server.URL + "/tool.bin",
			Sha256:       sha256OfBytes(content),
			DownloadPath: "tool.bin",
		}}},
	}, cache.root, "linux", "amd64")

	// When
	path, err := resolver.Resolve(context.Background(), "tool")
	if err != nil {
		t.Fatalf("resolve with earlier cache schema: %v", err)
	}

	// Then
	resolved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read resolved artifact: %v", err)
	}
	if string(resolved) != string(content) {
		t.Fatalf("resolved content = %q, want %q", resolved, content)
	}
	if downloads.Load() != 1 {
		t.Fatalf("expected prior entry not to be reused, got %d downloads", downloads.Load())
	}
}

func TestCache_ReportsSupersededIndexVersion(t *testing.T) {
	t.Parallel()

	cache := newIsolatedCache(t)
	writeIndexFixture(t, cache, `{"version":1,"entries":{}}`)

	if got := cache.SupersededIndexVersion(); got != 1 {
		t.Fatalf("expected the superseded version to be reported, got %d", got)
	}
}

func TestCache_RefusesNewerSchemaVersionWithGuidance(t *testing.T) {
	t.Parallel()

	cache := newIsolatedCache(t)
	writeIndexFixture(t, cache, `{"version":99,"entries":{}}`)

	_, err := cache.readIndex()
	if err == nil {
		t.Fatal("expected a newer schema to be refused")
	}
	if !strings.Contains(err.Error(), "newer version") ||
		!strings.Contains(err.Error(), "upgrade Exasol Personal") ||
		!strings.Contains(err.Error(), "cache clean --all") {
		t.Fatalf("expected the error to say how to recover, got %v", err)
	}
}

func TestResolver_InterruptedFetchLeavesNoUsableEntry(t *testing.T) {
	t.Parallel()

	var attempts int
	handler := func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			// Truncation forces checksum failure.
			_, _ = writer.Write([]byte("partial"))

			return
		}
		_, _ = writer.Write([]byte("payload"))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	cacheDir := t.TempDir()
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	if _, err := resolveTestDefinition(context.Background(), resolver, def, "payload"); err == nil {
		t.Fatal("expected the first attempt to fail")
	}

	index, err := resolver.cache.readIndex()
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("expected no entry recorded, got %d", len(index.Entries))
	}
	entries, err := os.ReadDir(filepath.Join(cacheDir, artifactsDirName))
	if err == nil && len(entries) != 0 {
		t.Fatalf("expected no entry directory, got %v", entries)
	}

	path, err := resolveTestDefinition(context.Background(), resolver, def, "payload")
	if err != nil {
		t.Fatalf("expected the second attempt to succeed, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the artifact to exist, got %v", err)
	}
}

func TestCache_CleanupReclaimsInterruptedEntries(t *testing.T) {
	t.Parallel()

	// Given
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, filepath.Join(cacheDir, cacheConfigFileName))
	stagingDir := stageIncompleteEntry(t, cache)
	index := emptyCacheIndex()
	seedCacheEntry(t, cache, &index, "complete", "payload", checksumString("payload"), testNow())
	writeTestIndex(t, cache, index)

	// When
	opts := CleanOptions{Mode: CleanupModeIncomplete}
	summary, err := cache.Clean(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if summary.RemovedEntries != 1 {
		t.Fatalf("expected one interrupted entry removed, got %+v", summary)
	}
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the staged entry to be gone, got %v", err)
	}
	if _, err := os.Stat(cache.entryDir("complete")); err != nil {
		t.Fatalf("complete entry was removed: %v", err)
	}
}

func TestCache_CleanupPreviewsInterruptedEntriesWithoutMutation(t *testing.T) {
	t.Parallel()

	// Given
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, filepath.Join(cacheDir, cacheConfigFileName))
	stagingDir := stageIncompleteEntry(t, cache)
	index := emptyCacheIndex()
	seedCacheEntry(t, cache, &index, "complete", "payload", checksumString("payload"), testNow())
	writeTestIndex(t, cache, index)
	indexBefore, err := os.ReadFile(cache.IndexPath())
	if err != nil {
		t.Fatalf("read index: %v", err)
	}

	// When
	summary, err := cache.Clean(context.Background(), CleanOptions{
		Mode: CleanupModeIncomplete, DryRun: true,
	})
	// Then
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if !summary.DryRun || summary.RemovedEntries != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if _, err := os.Stat(stagingDir); err != nil {
		t.Fatalf("preview removed staged entry: %v", err)
	}
	indexAfter, err := os.ReadFile(cache.IndexPath())
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !bytes.Equal(indexAfter, indexBefore) {
		t.Fatal("preview changed cache metadata")
	}
}

func stageIncompleteEntry(t *testing.T, cache *Cache) string {
	t.Helper()

	stagingDir, err := cache.newStagingDir()
	if err != nil {
		t.Fatalf("staging dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stagingDir, "partial.bin"), []byte("abc"), filePerm,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	return stagingDir
}
