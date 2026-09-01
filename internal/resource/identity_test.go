// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A declared checksum is a content hash, so the cache keys on it directly.
func TestContentIdentity_PrefersDeclaredChecksum(t *testing.T) {
	t.Parallel()

	const declared = "AbCdEf0123456789"
	got := contentIdentity(declared, "git-commit:deadbeef")
	if want := "sha256:abcdef0123456789"; got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

// Without a declared checksum the source's own answer identifies the content.
func TestContentIdentity_FallsBackToProbe(t *testing.T) {
	t.Parallel()

	if got := contentIdentity("", "git-commit:deadbeef"); got != "git-commit:deadbeef" {
		t.Fatalf("identity = %q, want the probed value", got)
	}
}

// Neither source means nothing identifies the content, which is what makes the
// resource refresh on every request.
func TestContentIdentity_EmptyWhenNothingIdentifiesContent(t *testing.T) {
	t.Parallel()

	if got := contentIdentity("   ", ""); got != "" {
		t.Fatalf("identity = %q, want empty", got)
	}
}

// A source that cannot be reached to state an identity must surface that,
// rather than silently degrading to an unidentified resource.
func TestManager_ReportsIdentityResolutionFailure(t *testing.T) {
	t.Parallel()

	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file:///nonexistent/probe/path"},
		},
	}

	_, err := manager.Get(context.Background(), def, "preset")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected the probe failure to be reported, got %v", err)
	}
}

// A source that states an identity lets the cache reuse its copy without
// transferring the content again.
func TestManager_ProbedIdentityAvoidsSecondTransfer(t *testing.T) {
	t.Parallel()

	var downloads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloads++
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	first, err := manager.Get(context.Background(), def, "payload")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := manager.Get(context.Background(), def, "payload")
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

// Content that does not match the declared checksum is rejected, and nothing
// is left behind that a later request would treat as cached.
func TestManager_RejectsAndDoesNotRecordMismatchedContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not what was declared"))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	if _, err := manager.Get(context.Background(), def, "payload"); err == nil {
		t.Fatal("expected a checksum mismatch error")
	}

	index, err := manager.cache.readIndex()
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("expected no recorded cache entry, got %d", len(index.Entries))
	}
}

// A local directory is used where it already is, so the cache records nothing
// for it.
func TestManager_LocalDirectoryOccupiesNoCacheEntry(t *testing.T) {
	t.Parallel()

	presetDir := t.TempDir()
	manifest := filepath.Join(presetDir, "infrastructure.yaml")
	if err := os.WriteFile(manifest, nil, filePerm); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{anyPlatformKey: {URL: "file://" + presetDir}},
	}

	got, err := manager.Get(context.Background(), def, "preset")
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

// Entries created under an earlier schema describe a layout this launcher no
// longer reads, so they are dropped rather than reused, and the launcher keeps
// working instead of failing on every request.
func TestManager_StartsFreshOnPriorCacheSchemaVersion(t *testing.T) {
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
	manager := NewResourceManagerForPlatform(ResourceSpec{
		"tool": {Artifact: map[string]ArtifactSpec{anyPlatformKey: {
			URL:          server.URL + "/tool.bin",
			Sha256:       sha256OfBytes(content),
			DownloadPath: "tool.bin",
		}}},
	}, cache.root, "linux", "amd64")

	// When
	path, err := manager.Request(context.Background(), "tool")
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

// The superseded content stays on disk, so the launcher can tell the user how
// to reclaim its space.
func TestCache_ReportsSupersededIndexVersion(t *testing.T) {
	t.Parallel()

	cache := newIsolatedCache(t)
	writeIndexFixture(t, cache, `{"version":1,"entries":{}}`)

	if got := cache.SupersededIndexVersion(); got != 1 {
		t.Fatalf("expected the superseded version to be reported, got %d", got)
	}
}

// An index from a newer launcher cannot be interpreted, so it is refused, and
// the refusal says what to do about it.
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

// An interrupted materialization must leave nothing that a later request
// mistakes for a complete entry.
func TestManager_InterruptedFetchLeavesNoUsableEntry(t *testing.T) {
	t.Parallel()

	var attempts int
	handler := func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			// Truncated body: the checksum will not match.
			_, _ = writer.Write([]byte("partial"))

			return
		}
		_, _ = writer.Write([]byte("payload"))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	cacheDir := t.TempDir()
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    server.URL + "/payload.bin",
				Sha256: sha256OfBytes([]byte("payload")),
			},
		},
	}

	if _, err := manager.Get(context.Background(), def, "payload"); err == nil {
		t.Fatal("expected the first attempt to fail")
	}

	index, err := manager.cache.readIndex()
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

	// A later request materializes the resource for real.
	path, err := manager.Get(context.Background(), def, "payload")
	if err != nil {
		t.Fatalf("expected the second attempt to succeed, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the artifact to exist, got %v", err)
	}
}

// Space used by an interrupted materialization stays reclaimable.
func TestCache_CleanupReclaimsInterruptedEntries(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, filepath.Join(cacheDir, cacheConfigFileName))

	stagingDir, err := cache.newStagingDir()
	if err != nil {
		t.Fatalf("staging dir: %v", err)
	}
	partial := filepath.Join(stagingDir, "partial.bin")
	if err := os.WriteFile(partial, []byte("abc"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}

	opts := CleanOptions{Mode: CleanupModeIncomplete}
	summary, err := cache.Clean(context.Background(), opts)
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if summary.RemovedEntries != 1 {
		t.Fatalf("expected one interrupted entry removed, got %+v", summary)
	}
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the staged entry to be gone, got %v", err)
	}
}
