// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestParseSpec_RoundTripsEmbedField(t *testing.T) {
	t.Parallel()

	// Given
	raw := []byte(`
embedded:
  extract: true
  embed: true
  artifact:
    linux/amd64:
      url: https://example.com/embedded-linux.tar.gz
      sha256: deadbeef
      subpath: tool
not-embedded:
  extract: true
  artifact:
    linux/amd64:
      url: https://example.com/not-embedded-linux.tar.gz
      sha256: deadbeef
      subpath: tool
`)

	// When
	spec, err := ParseSpec(raw)
	// Then
	if err != nil {
		t.Fatalf("expected spec to be valid, got %v", err)
	}
	if spec["embedded"].Embed != EmbedDefault {
		t.Fatal("expected embed: true to round-trip as EmbedDefault")
	}
	if spec["not-embedded"].Embed != EmbedNever {
		t.Fatal("expected omitted embed field to default to EmbedNever")
	}
}

func TestParseSpec_AllowsResourcePathWithoutExtraction(t *testing.T) {
	t.Parallel()

	// Given — subpath selects a subpath inside whatever the source
	// produces, and is valid regardless of source kind or extract flag.
	raw := []byte(`
artifact:
  extract: false
  artifact:
    linux/amd64:
      url: https://example.com/artifact-linux.tar.gz
      sha256: deadbeef
      subpath: tofu
`)

	// When
	spec, err := ParseSpec(raw)
	// Then
	if err != nil {
		t.Fatalf("expected subpath without extraction to be valid, got %v", err)
	}
	if got := spec["artifact"].Artifact["linux/amd64"].Subpath; got != "tofu" {
		t.Fatalf("expected subpath %q, got %q", "tofu", got)
	}
}

func TestParseSpec_GitURLWithRefIsRecognisedAsGitSource(t *testing.T) {
	t.Parallel()

	// Given — an ArtifactSpec.URL for a git source that still carries an @ref
	// suffix (the shape produced by ResolvePreset when a user passes
	// `repo.git@v1#subpath`). The spec must classify it as a git source so
	// sha256 is not required.
	raw := []byte(`
preset:
  extract: false
  artifact:
    any:
      url: https://example.com/repo.git@v1
`)

	// When
	spec, err := ParseSpec(raw)
	// Then
	if err != nil {
		t.Fatalf("expected git URL with @ref to be recognised as a git source, got %v", err)
	}
	wantURL := "https://example.com/repo.git@v1"
	if got := spec["preset"].Artifact[anyPlatformKey].URL; got != wantURL {
		t.Fatalf("expected URL to round-trip verbatim, got %q", got)
	}
}

func TestManager_ResolveEntry_HonoursResourcePathWithoutExtraction(t *testing.T) {
	t.Parallel()

	// Given — a non-extract resource with a subpath. The manager must
	// apply the subpath to the artifact path regardless of source kind.
	cacheDir := t.TempDir()
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "https://example.com/repo.git", Subpath: "infra/aws"},
		},
	}

	// When
	entry, err := manager.resolveEntry("preset", def, def.Artifact[anyPlatformKey], "sha256:test")
	// Then
	if err != nil {
		t.Fatalf("expected resolveEntry to succeed, got %v", err)
	}
	// The download path is the URL basename; the resolved path must point at
	// the subdirectory joined onto it.
	wantSuffix := filepath.Join("repo.git", "infra", "aws")
	if !strings.HasSuffix(entry.ResolvedPath, wantSuffix) {
		t.Fatalf("expected ResolvedPath to end with %q, got %q", wantSuffix, entry.ResolvedPath)
	}
	if entry.Subpath != "infra/aws" {
		t.Fatalf("expected Subpath %q, got %q", "infra/aws", entry.Subpath)
	}
}

func TestManager_ResolveEntry_RejectsTraversalResourcePathWithoutExtraction(t *testing.T) {
	t.Parallel()

	// Given
	cacheDir := t.TempDir()
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "https://example.com/repo.git", Subpath: "../escape"},
		},
	}

	// When
	_, err := manager.resolveEntry("preset", def, def.Artifact[anyPlatformKey], "sha256:test")
	// Then
	if err == nil || !strings.Contains(err.Error(), "subpath") {
		t.Fatalf("expected subpath traversal error, got %v", err)
	}
}

func TestManager_ResolveEntry_DifferentSubpathsProduceDistinctEntries(t *testing.T) {
	t.Parallel()

	// Given
	cacheDir := t.TempDir()
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")
	makeDef := func(subpath string) ResourceDefinition {
		return ResourceDefinition{
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {URL: "https://example.com/repo.git", Subpath: subpath},
			},
		}
	}

	// When
	defA := makeDef("infra/aws")
	defB := makeDef("infra/azure")
	artifactA, artifactB := defA.Artifact[anyPlatformKey], defB.Artifact[anyPlatformKey]
	entryA, errA := manager.resolveEntry("preset", defA, artifactA, "sha256:test")
	entryB, errB := manager.resolveEntry("preset", defB, artifactB, "sha256:test")
	// Then
	if errA != nil || errB != nil {
		t.Fatalf("expected both resolveEntry calls to succeed, got %v / %v", errA, errB)
	}
	if entryA.Key == entryB.Key {
		t.Fatalf("expected distinct cache keys for different subpaths, both were %q", entryA.Key)
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
	assertPathInCache(
		t,
		deploymentDir,
		path,
		"artifact",
		filepath.Join("linux", "amd64"),
		"artifact.tar.gz",
	)
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
					URL:     server.URL + "/artifact.tgz",
					Sha256:  sum,
					Subpath: "tool",
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
	assertPathInCache(
		t,
		deploymentDir,
		path1,
		"artifact",
		filepath.Join("linux", "amd64"),
		filepath.Join("unpack", "tool"),
	)
	if _, err := os.Stat(path1); err != nil {
		t.Fatalf("expected resolved path to exist, got %v", err)
	}
	readmePath := filepath.Join(filepath.Dir(path1), "nested", "README")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("expected extracted nested README to exist, got %v", err)
	}
	configPath := filepath.Join(filepath.Dir(path1), "nested", "config", "x")
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
	index, _, err := manager.cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read cache index: %v", err)
	}
	entry := onlyCacheEntry(t, index)
	expectedSize, err := directorySize(manager.cache.absolutePath(entry.EntryPath))
	if err != nil {
		t.Fatalf("failed to calculate cache entry size: %v", err)
	}
	if entry.SizeBytes != expectedSize || entry.SizeBytes <= int64(len(archiveData)) {
		t.Fatalf(
			"expected size to include archive and extracted contents, got %d; archive size is %d",
			entry.SizeBytes,
			len(archiveData),
		)
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

func TestManager_RequestExtractsZipResource(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	archivePath := writeZipFixture(
		t,
		deploymentDir,
		"artifact.zip",
		map[string]string{
			"launcher":      "runner",
			"nested/README": "readme",
		},
	)
	sum := sha256OfTestFile(t, archivePath)
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	server := newArtifactServer(t, "artifact.zip", archiveData)
	spec := ResourceSpec{
		"artifact": {
			Extract: true,
			Artifact: map[string]ArtifactSpec{
				"darwin/arm64": {
					URL:          server.URL + "/artifact.zip",
					Sha256:       sum,
					DownloadPath: "artifact.zip",
					Subpath:      "launcher",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "darwin", "arm64")

	// When
	path, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertPathInCache(
		t,
		deploymentDir,
		path,
		"artifact",
		filepath.Join("darwin", "arm64"),
		filepath.Join("unpack", "launcher"),
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected resolved path to be readable, got %v", err)
	}
	if string(data) != "runner" {
		t.Fatalf("expected resolved resource content, got %q", string(data))
	}
}

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestManager_RequestResolvesEmbeddedResourceWithoutNetwork(t *testing.T) {
	// Given
	const resourceID = "embedded-resolve-test"
	deploymentDir := t.TempDir()
	fixtureDir := t.TempDir()
	archivePath := writeZipFixture(t, fixtureDir, "artifact.zip", map[string]string{
		"launcher": "runner",
	})
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	Register(resourceID, archiveData, sha256OfTestFile(t, archivePath))
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})

	server, requests := newCountingArtifactServer(t, "artifact.zip", archiveData)
	spec := ResourceSpec{
		resourceID: {
			Extract: true,
			Embed:   EmbedDefault,
			Artifact: map[string]ArtifactSpec{
				"darwin/arm64": {
					URL:     server.URL + "/artifact.zip",
					Sha256:  sha256OfTestFile(t, archivePath),
					Subpath: "launcher",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "darwin", "arm64")

	// When
	path, err := manager.Request(context.Background(), resourceID)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("expected embedded resolution to never contact the network, got %d requests",
			requests.Load())
	}
	// Same cache layout (unpack/launcher) a network-fetched zip of this shape
	// would produce — proves extraction reused the identical code path.
	assertPathInCache(
		t,
		deploymentDir,
		path,
		resourceID,
		filepath.Join("darwin", "arm64"),
		filepath.Join("unpack", "launcher"),
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected resolved path to be readable, got %v", err)
	}
	if string(data) != "runner" {
		t.Fatalf("expected resolved resource content, got %q", string(data))
	}
}

func TestManager_RequestFailsWhenEmbeddedResourceNotRegistered(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	server, requests := newCountingArtifactServer(t, "artifact.zip", []byte("unused"))
	spec := ResourceSpec{
		"embedded-missing-test": {
			Extract: true,
			Embed:   EmbedDefault,
			Artifact: map[string]ArtifactSpec{
				"darwin/arm64": {
					URL:     server.URL + "/artifact.zip",
					Sha256:  "deadbeef",
					Subpath: "launcher",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "darwin", "arm64")

	// When
	_, err := manager.Request(context.Background(), "embedded-missing-test")

	// Then
	if err == nil || !strings.Contains(err.Error(), "no embedded data registered") {
		t.Fatalf("expected a hard error for a missing embedded registration, got %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("expected no fallback to network sources, got %d requests", requests.Load())
	}
}

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestManager_EmbeddedResourceIdentityComesFromContentHash(t *testing.T) {
	// Given: one cache root and spec shared across two "builds" that embed
	// different content under the same resource ID.
	const resourceID = "embedded-identity-test"
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})
	spec := ResourceSpec{
		resourceID: {
			Embed: EmbedDefault,
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {URL: "embedded://" + resourceID},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")

	// When: the first build resolves its own embedded content.
	firstData := []byte("build-one-content")
	Register(resourceID, firstData, sha256OfBytes(firstData))
	firstPath, err := manager.Request(context.Background(), resourceID)
	if err != nil {
		t.Fatalf("expected no error resolving the first payload, got %v", err)
	}
	firstResolved, err := os.ReadFile(firstPath)
	if err != nil || string(firstResolved) != string(firstData) {
		t.Fatalf("expected the first payload's own content, got %q, err %v", firstResolved, err)
	}

	// When: a second build, sharing the same cache root and resource ID,
	// registers different content (simulating an upgraded binary).
	secondData := []byte("build-two-content")
	Register(resourceID, secondData, sha256OfBytes(secondData))
	secondPath, err := manager.Request(context.Background(), resourceID)
	if err != nil {
		t.Fatalf("expected no error resolving the second payload, got %v", err)
	}
	secondResolved, err := os.ReadFile(secondPath)
	if err != nil || string(secondResolved) != string(secondData) {
		t.Fatalf("expected the second payload's own content, got %q, err %v", secondResolved, err)
	}

	// Then: the two payloads resolved to distinct cache entries, and the
	// first entry's content was never overwritten by the second.
	if firstPath == secondPath {
		t.Fatalf(
			"expected a different content hash to resolve to a different entry, got %q twice",
			firstPath,
		)
	}
	firstStillIntact, err := os.ReadFile(firstPath)
	if err != nil || string(firstStillIntact) != string(firstData) {
		t.Fatalf("expected the first entry to remain intact, got %q, err %v", firstStillIntact, err)
	}
}

//nolint:paralleltest // embeddedResources is shared; concurrent Register calls aren't safe.
func TestManager_RequestIgnoresEmbeddedRegistryWithoutEmbedFlag(t *testing.T) {
	// Given
	const resourceID = "embedded-ignored-test"
	deploymentDir := t.TempDir()
	Register(resourceID, []byte("should never be used"), "unused-hash")
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})

	content := []byte("from the network")
	server := newArtifactServer(t, "artifact.bin", content)
	sum := sha256.Sum256(content)
	spec := ResourceSpec{
		resourceID: {
			Artifact: map[string]ArtifactSpec{
				"darwin/arm64": {
					URL:          server.URL + "/artifact.bin",
					Sha256:       hex.EncodeToString(sum[:]),
					DownloadPath: "artifact.bin",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "darwin", "arm64")

	// When
	path, err := manager.Request(context.Background(), resourceID)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected resolved path to be readable, got %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("expected network-fetched content, got %q", string(data))
	}
}

func TestManager_RequestUpdatesLastUsedAtOnCacheHit(t *testing.T) {
	t.Parallel()

	// Given
	clock := &testClock{now: testNow()}
	cache := newCacheWithClock(
		filepath.Join(t.TempDir(), "cache"),
		filepath.Join(t.TempDir(), cacheConfigFileName),
		clock,
	)
	data := []byte("artifact")
	server, requests := newCountingArtifactServer(t, "artifact.bin", data)
	spec := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.bin",
					Sha256:       checksumString(string(data)),
					DownloadPath: "artifact.bin",
				},
			},
		},
	}
	manager := NewResourceManagerWithCacheForPlatform(spec, cache, "linux", "amd64")

	// When
	path1, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	index, _, err := cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read cache index: %v", err)
	}
	entry := onlyCacheEntry(t, index)
	if !entry.CreatedAt.Equal(clock.now) || !entry.LastUsedAt.Equal(clock.now) {
		t.Fatalf("unexpected initial timestamps: %+v", entry)
	}

	// When
	clock.now = clock.now.Add(2 * time.Hour)
	path2, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected cache hit to succeed, got %v", err)
	}
	if path2 != path1 {
		t.Fatalf("expected cache hit to reuse %q, got %q", path1, path2)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected cache hit to avoid a second download, got %d requests", requests.Load())
	}
	index, _, err = cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read cache index: %v", err)
	}
	entry = onlyCacheEntry(t, index)
	if !entry.CreatedAt.Equal(testNow()) {
		t.Fatalf("expected created timestamp to remain unchanged, got %s", entry.CreatedAt)
	}
	if !entry.LastUsedAt.Equal(clock.now) {
		t.Fatalf("expected last-used timestamp %s, got %s", clock.now, entry.LastUsedAt)
	}
}

func TestManager_RequestRefreshesMissingCachedFile(t *testing.T) {
	t.Parallel()

	// Given
	clock := &testClock{now: testNow()}
	cache := newCacheWithClock(
		filepath.Join(t.TempDir(), "cache"),
		filepath.Join(t.TempDir(), cacheConfigFileName),
		clock,
	)
	data := []byte("artifact")
	server, requests := newCountingArtifactServer(t, "artifact.bin", data)
	spec := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.bin",
					Sha256:       checksumString(string(data)),
					DownloadPath: "artifact.bin",
				},
			},
		},
	}
	manager := NewResourceManagerWithCacheForPlatform(spec, cache, "linux", "amd64")
	path1, err := manager.Request(context.Background(), "artifact")
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	if err := os.Remove(path1); err != nil {
		t.Fatalf("failed to remove cached artifact: %v", err)
	}

	// When
	clock.now = clock.now.Add(time.Hour)
	path2, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected missing file refresh to succeed, got %v", err)
	}
	if path2 != path1 {
		t.Fatalf("expected refresh to reuse identity path %q, got %q", path1, path2)
	}
	if requests.Load() != 2 {
		t.Fatalf(
			"expected missing file to trigger a second download, got %d requests",
			requests.Load(),
		)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Fatalf("expected refreshed artifact file, got %v", err)
	}
}

func TestManager_RequestRunsAutomaticStaleCleanupWhenDue(t *testing.T) {
	t.Parallel()

	// Given
	clock := &testClock{now: testNow()}
	cache := newCacheWithClock(
		filepath.Join(t.TempDir(), "cache"),
		filepath.Join(t.TempDir(), cacheConfigFileName),
		clock,
	)
	writeTestCacheConfig(t, cache, 1)
	index := emptyCacheIndex()
	stale := seedCacheEntry(
		t,
		cache,
		&index,
		"stale",
		"old",
		checksumString("old"),
		clock.now.AddDate(0, 0, -10),
	)
	writeTestIndex(t, cache, index)
	data := []byte("artifact")
	server := newArtifactServer(t, "artifact.bin", data)
	spec := ResourceSpec{
		"artifact": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/artifact.bin",
					Sha256:       checksumString(string(data)),
					DownloadPath: "artifact.bin",
				},
			},
		},
	}
	manager := NewResourceManagerWithCacheForPlatform(spec, cache, "linux", "amd64")

	// When
	_, err := manager.Request(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected request to succeed, got %v", err)
	}
	if _, err := os.Stat(cache.absolutePath(stale.EntryPath)); !os.IsNotExist(err) {
		t.Fatalf("expected stale entry to be removed, got %v", err)
	}
	read, _, err := cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read index: %v", err)
	}
	if _, ok := read.Entries["stale"]; ok {
		t.Fatal("expected stale metadata to be removed")
	}
	if len(read.Entries) != 1 {
		t.Fatalf("expected only refreshed artifact metadata, got %+v", read.Entries)
	}
	if !read.LastCleanup.Equal(clock.now) {
		t.Fatalf("expected automatic cleanup timestamp %s, got %s", clock.now, read.LastCleanup)
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
					URL:     server.URL + "/artifact.tgz",
					Sha256:  strings.Repeat("0", 64),
					Subpath: "tool",
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
	if path2 == path1 {
		t.Fatalf("expected changed checksum to use a new cache path, got %q", path2)
	}
	data, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("expected refreshed artifact to be readable, got %v", err)
	}
	if string(data) != string(secondData) {
		t.Fatal("expected refreshed artifact content to change")
	}
}

func assertPathInCache(
	t *testing.T,
	cacheRoot, actualPath, resourceID, platformDir, suffix string,
) {
	t.Helper()

	prefix := filepath.Join(cacheRoot, artifactsDirName, resourceID, platformDir)
	if !strings.HasPrefix(actualPath, prefix+string(filepath.Separator)) {
		t.Fatalf("expected path under %q, got %q", prefix, actualPath)
	}
	if !strings.HasSuffix(actualPath, suffix) {
		t.Fatalf("expected path to end with %q, got %q", suffix, actualPath)
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
	slices.Sort(keys)
	for _, entryName := range keys {
		writeTarGzFixtureEntry(t, tarWriter, entryName, entries[entryName])
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

func writeTarGzFixtureEntry(
	t *testing.T,
	tarWriter *tar.Writer,
	entryName string,
	entry tarFixtureEntry,
) {
	t.Helper()

	if entry.IsDir || strings.HasSuffix(entryName, "/") {
		if err := tarWriter.WriteHeader(&tar.Header{
			Name:     entryName,
			Mode:     entry.Mode,
			Typeflag: tar.TypeDir,
		}); err != nil {
			t.Fatalf("failed to write tar directory header: %v", err)
		}

		return
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

func writeZipFixture(t *testing.T, dir, name string, entries map[string]string) string {
	t.Helper()
	return writeZipFixtureEntries(t, dir, name, entries)
}

func sha256OfBytes(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
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

func newCountingArtifactServer(
	t *testing.T,
	artifactName string,
	data []byte,
) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	requests := &atomic.Int64{}
	handler := func(writer http.ResponseWriter, request *http.Request) {
		// Only transfers count: a validator request moves no artifact bytes.
		if request.Method != http.MethodHead {
			requests.Add(1)
		}
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

	return server, requests
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

func onlyCacheEntry(t *testing.T, index cacheIndex) cacheIndexEntry {
	t.Helper()

	if len(index.Entries) != 1 {
		t.Fatalf("expected one cache entry, got %+v", index.Entries)
	}
	for _, entry := range index.Entries {
		return entry
	}

	t.Fatal("expected one cache entry")

	return cacheIndexEntry{}
}

func TestManager_GetWithRuntimeDefinition(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	data := []byte("tool-binary")
	server := newArtifactServer(t, "tool.bin", data)
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:          server.URL + "/tool.bin",
				Sha256:       checksumString(string(data)),
				DownloadPath: "tool.bin",
			},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, deploymentDir, "linux", "amd64")

	// When
	path, err := manager.Get(context.Background(), def, "tool-binary")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected resolved path to exist, got %v", err)
	}
}

func TestParseSpec_RejectsArchiveWithMissingChecksum(t *testing.T) {
	t.Parallel()

	raw := []byte(`
myresource:
  extract: false
  artifact:
    linux/amd64:
      url: https://example.com/tool.tar.gz
      sha256: ""
`)

	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "must define sha256") {
		t.Fatalf("expected missing sha256 error, got %v", err)
	}
}

func TestManager_GetNoChecksumAlwaysRefetches(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	data := []byte("artifact-content")
	server, requests := newCountingArtifactServer(t, "tool.bin", data)
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:          server.URL + "/tool.bin",
				Sha256:       "",
				DownloadPath: "tool.bin",
			},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, deploymentDir, "linux", "amd64")

	// When
	_, err := manager.Get(context.Background(), def, "tool-binary")
	if err != nil {
		t.Fatalf("expected first Get to succeed, got %v", err)
	}
	_, err = manager.Get(context.Background(), def, "tool-binary")
	if err != nil {
		t.Fatalf("expected second Get to succeed, got %v", err)
	}
	// Then
	if requests.Load() != 2 {
		t.Fatalf("expected 2 downloads for no-checksum archive, got %d", requests.Load())
	}
}

//nolint:paralleltest // embeddedHashes/embeddedResources are shared globals.
func TestManager_GetEmbeddedResourcePrefersRegisteredHashOverADeclaredChecksum(t *testing.T) {
	// Given: an embedded resource whose artifact also declares a checksum,
	// as tofu's local-runner and nano-image resources already do — the
	// declared checksum verifies what got fetched, not necessarily what a
	// given build ends up embedding.
	const resourceID = "manager-test-embed-checksum-fix"
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})
	def := ResourceDefinition{
		Embed: EmbedDefault,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:    "https://example.invalid/archive.tar.gz",
				Sha256: "source-archive-checksum",
			},
		},
	}
	cacheRoot := t.TempDir()

	// When: a first build registers one set of embedded bytes...
	Register(resourceID, []byte("old-content"), "hash-of-old-content")
	oldManager := NewResourceManagerForPlatform(ResourceSpec{}, cacheRoot, "linux", "amd64")
	oldPath, err := oldManager.Get(context.Background(), def, resourceID)
	if err != nil {
		t.Fatalf("expected first Get to succeed, got %v", err)
	}
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("failed to read first resolved artifact: %v", err)
	}

	// ...and a later build registers different bytes, while the source
	// artifact's own declared checksum stays the same.
	Register(resourceID, []byte("new-content"), "hash-of-new-content")
	newManager := NewResourceManagerForPlatform(ResourceSpec{}, cacheRoot, "linux", "amd64")
	newPath, err := newManager.Get(context.Background(), def, resourceID)
	if err != nil {
		t.Fatalf("expected second Get to succeed, got %v", err)
	}
	newData, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read second resolved artifact: %v", err)
	}

	// Then: the second build resolves its own embedded bytes, not a stale
	// cache entry reused because the declared checksum never changed.
	if string(oldData) != "old-content" {
		t.Fatalf("expected the first build's own embedded content, got %q", oldData)
	}
	if string(newData) != "new-content" {
		t.Fatalf(
			"expected the second build's own embedded content, got %q (stale cache entry reused)",
			newData,
		)
	}
}

func TestManager_GetGitSourceCachedOnSameCommit(t *testing.T) {
	t.Parallel()

	// Given
	repoDir, _ := createTestGitRepo(t, map[string]string{"preset.txt": "content"})
	cacheDir := t.TempDir()
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: repoDir},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")

	// When — first fetch clones the repo
	path, err := manager.Get(context.Background(), def, "preset")
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}

	// Corrupt a file in the cache to detect whether Fetch is called again.
	corruptedFile := filepath.Join(path, "preset.txt")
	if err := os.WriteFile(corruptedFile, []byte("corrupted"), filePerm); err != nil {
		t.Fatalf("corrupt failed: %v", err)
	}

	// When — second Get with same commit; Identify returns same hash → cache hit
	path2, err := manager.Get(context.Background(), def, "preset")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}

	// Then — same path returned, Fetch was not called (corrupted content preserved)
	if path != path2 {
		t.Fatalf("expected same cache path, got %q vs %q", path, path2)
	}
	got, err := os.ReadFile(corruptedFile)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != "corrupted" {
		t.Fatalf("expected cache hit (corrupted content preserved), got %q", string(got))
	}
}

func TestManager_GetGitSourceRefetchesOnNewCommit(t *testing.T) {
	t.Parallel()

	// Given
	repoDir, _ := createTestGitRepo(t, map[string]string{"v1.txt": "v1"})
	cacheDir := t.TempDir()
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: repoDir},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")

	_, err := manager.Get(context.Background(), def, "preset")
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}

	// Advance the remote
	addCommitToTestRepo(t, repoDir, "v2.txt", "v2")

	// When — second Get with new commit
	path, err := manager.Get(context.Background(), def, "preset")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}

	// Then — new content is present
	if _, err := os.Stat(filepath.Join(path, "v2.txt")); err != nil {
		t.Fatalf("expected v2.txt after re-fetch, got %v", err)
	}
}

func TestManager_GetFileDirectoryReturnedDirectly(t *testing.T) {
	t.Parallel()

	// Given
	presetDir := t.TempDir()
	cacheDir := t.TempDir()
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file://" + presetDir},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := manager.Get(context.Background(), def, "preset-dir")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Fetch returns a redirect path for local directories; the Manager records it
	// in the cache index but does not write an artifact to the cache directory.
	if strings.HasPrefix(path, cacheDir) {
		t.Fatalf("expected original path, not a cache path; got %q", path)
	}
	if path != presetDir {
		t.Fatalf("expected path %q, got %q", presetDir, path)
	}
}

func TestManager_GetFileDirectoryWithResourcePathReturnsSubdirectory(t *testing.T) {
	t.Parallel()

	// Given — a preset root with a nested subdirectory. The manager must apply
	// subpath to the redirect target, not silently return the root.
	presetDir := t.TempDir()
	subDir := filepath.Join(presetDir, "infra", "aws")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatalf("failed to create sub directory: %v", err)
	}
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file://" + presetDir, Subpath: "infra/aws"},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	path, err := manager.Get(context.Background(), def, "preset-dir")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != subDir {
		t.Fatalf("expected path %q, got %q", subDir, path)
	}
}

func TestManager_GetFileDirectoryRejectsTraversalResourcePath(t *testing.T) {
	t.Parallel()

	// Given — a subpath that tries to escape the redirect root.
	presetDir := t.TempDir()
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file://" + presetDir, Subpath: "../escape"},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	_, err := manager.Get(context.Background(), def, "preset-dir")
	// Then
	if err == nil || !strings.Contains(err.Error(), "subpath") {
		t.Fatalf("expected subpath traversal error, got %v", err)
	}
}

func TestManager_GetFileBareFileReturnedDirectly(t *testing.T) {
	t.Parallel()

	// Given
	binaryPath := filepath.Join(t.TempDir(), "launcher")
	if err := os.WriteFile(binaryPath, []byte("binary"), filePerm); err != nil {
		t.Fatalf("write launcher fixture: %v", err)
	}
	cacheDir := t.TempDir()
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file://" + binaryPath},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := manager.Get(context.Background(), def, "local-launcher")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != binaryPath {
		t.Fatalf("expected path %q, got %q", binaryPath, path)
	}
}

func TestManager_GetFileDirectoryMissingReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file:///nonexistent/path/to/preset"},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	_, err := manager.Get(context.Background(), def, "preset-missing")
	// Then
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}

func TestManager_GetFileArchiveExtractedIntoCache(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	archivePath := writeTarGzFixture(t, srcDir, "preset.tar.gz", "tool")
	cacheDir := t.TempDir()
	def := ResourceDefinition{
		Extract: true,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file://" + archivePath, Subpath: "tool"},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := manager.Get(context.Background(), def, "preset")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if strings.HasPrefix(path, srcDir) {
		t.Fatalf("expected path inside cache, got %q", path)
	}
	if !strings.HasPrefix(path, cacheDir) {
		t.Fatalf("expected path under cache root %q, got %q", cacheDir, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected extracted tool to exist, got %v", err)
	}
}

func TestManager_GetAnyPlatformFallback(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	data := []byte("cross-platform-tool")
	server := newArtifactServer(t, "tool.bin", data)
	spec := ResourceSpec{
		"tool": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {
					URL:          server.URL + "/tool.bin",
					Sha256:       checksumString(string(data)),
					DownloadPath: "tool.bin",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "darwin", "arm64")

	// When
	path, err := manager.Request(context.Background(), "tool")
	// Then
	if err != nil {
		t.Fatalf("expected any-platform fallback to succeed, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected resolved path to exist, got %v", err)
	}
}

func TestManager_GetPlatformSpecificTakesPriorityOverAny(t *testing.T) {
	t.Parallel()

	// Given: a definition with both a platform-specific key and "any"
	deploymentDir := t.TempDir()
	platformData := []byte("platform-specific-tool")
	anyData := []byte("any-platform-tool")
	server := newArtifactServer(t, "tool.bin", platformData)
	anyServer := newArtifactServer(t, "tool.bin", anyData)
	spec := ResourceSpec{
		"tool": {
			Extract: false,
			Artifact: map[string]ArtifactSpec{
				"linux/amd64": {
					URL:          server.URL + "/tool.bin",
					Sha256:       checksumString(string(platformData)),
					DownloadPath: "tool.bin",
				},
				anyPlatformKey: {
					URL:          anyServer.URL + "/tool.bin",
					Sha256:       checksumString(string(anyData)),
					DownloadPath: "tool.bin",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")

	// When
	path, err := manager.Request(context.Background(), "tool")
	// Then
	if err != nil {
		t.Fatalf("expected platform-specific resolution to succeed, got %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected resolved path to be readable, got %v", err)
	}
	if string(content) != string(platformData) {
		t.Fatalf("expected platform-specific artifact, got %q", string(content))
	}
}

func TestManager_GetZipExtraction(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	archivePath := writeZipArchiveFixture(t, srcDir, "preset.zip", "tool", "tool-content")
	cacheDir := t.TempDir()
	def := ResourceDefinition{
		Extract: true,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {
				URL:     "file://" + archivePath,
				Subpath: "tool",
			},
		},
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := manager.Get(context.Background(), def, "preset")
	// Then
	if err != nil {
		t.Fatalf("expected zip extraction to succeed, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected extracted file to be readable, got %v", err)
	}
	if string(data) != "tool-content" {
		t.Fatalf("expected %q, got %q", "tool-content", string(data))
	}
}

func writeZipArchiveFixture(t *testing.T, dir, archiveName, entryName, content string) string {
	t.Helper()
	return writeZipFixtureEntries(t, dir, archiveName, map[string]string{
		entryName: content,
	})
}

func TestGlobMatches_MatchesFilesAndDirectoriesAlike(t *testing.T) {
	t.Parallel()

	// Given: a plain-file match alongside a directory match — GlobMatches
	// makes no assumption that a match must be a directory.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "aws"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "azure.txt"), []byte("azure"), filePerm,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "README.md"), []byte("not a module"), filePerm,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}

	// When
	matches, err := GlobMatches(root, "*")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %v", matches)
	}
	if matches["aws"] != filepath.Join(root, "aws") {
		t.Fatalf("expected the aws subdirectory match, got %v", matches)
	}
	if matches["azure.txt"] != filepath.Join(root, "azure.txt") {
		t.Fatalf("expected the azure.txt file match, got %v", matches)
	}
}

func TestGlobMatches_MatchesWithinASubdirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "modules", "aws"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}

	matches, err := GlobMatches(root, "modules/*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if matches["aws"] != filepath.Join(root, "modules", "aws") {
		t.Fatalf("expected the nested aws match, got %v", matches)
	}
}

func TestGlobMatches_RejectsMatchesSharingAMemberName(t *testing.T) {
	t.Parallel()

	// Given: two directories under different parents share a basename, so a
	// pattern spanning both would otherwise silently drop one via overwrite.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "aws", "config"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "azure", "config"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}

	// When
	_, err := GlobMatches(root, "*/config")
	// Then
	if err == nil {
		t.Fatal("expected an error for a pattern matching duplicate member names, got none")
	}
}

func TestGlobMatches_ExcludesGitMetadataDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture .git directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "aws"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}

	matches, err := GlobMatches(root, "*")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := matches[".git"]; ok {
		t.Fatalf("expected .git to be excluded, got %v", matches)
	}
}

func TestGlobMatches_PropagatesAMissingPatternError(t *testing.T) {
	t.Parallel()

	if _, err := GlobMatches(t.TempDir(), ""); err == nil {
		t.Fatal("expected an error for an empty pattern, got none")
	}
}

func TestManager_RequestMember_ResolvesAMatchWithinALocalDirectory(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	for _, name := range []string{"aws", "azure"} {
		if err := os.MkdirAll(filepath.Join(srcDir, name), dirPerm); err != nil {
			t.Fatalf("failed to create fixture subdirectory: %v", err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(srcDir, "README.md"), []byte("not a module"), filePerm,
	); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}
	spec := ResourceSpec{
		"shared-modules": {
			Glob:     true,
			Artifact: map[string]ArtifactSpec{anyPlatformKey: {URL: srcDir, Subpath: "*"}},
		},
	}
	manager := NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")

	// When
	path, err := manager.RequestMember(context.Background(), "shared-modules", "aws")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != filepath.Join(srcDir, "aws") {
		t.Fatalf("expected the aws subdirectory path, got %q", path)
	}
}

func TestManager_RequestMember_ResolvesAMatchWithinAnExtractedArchive(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(t, srcDir, "presets.tar.gz", map[string]tarFixtureEntry{
		"aws/main.tf":   {Content: "aws module"},
		"azure/main.tf": {Content: "azure module"},
		"README.md":     {Content: "not a module"},
	})
	spec := ResourceSpec{
		"shared-modules": {
			Extract: true,
			Glob:    true,
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {URL: "file://" + archivePath, Subpath: "*"},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")

	// When
	path, err := manager.RequestMember(context.Background(), "shared-modules", "aws")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "main.tf")); err != nil {
		t.Fatalf("expected main.tf inside matched %q, got %v", path, err)
	}
}

func TestManager_RequestMember_ReturnsErrorForUnknownMember(t *testing.T) {
	t.Parallel()

	// Given
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "aws"), dirPerm); err != nil {
		t.Fatalf("failed to create fixture subdirectory: %v", err)
	}
	spec := ResourceSpec{
		"shared-modules": {
			Glob:     true,
			Artifact: map[string]ArtifactSpec{anyPlatformKey: {URL: srcDir, Subpath: "*"}},
		},
	}
	manager := NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")

	// When
	_, err := manager.RequestMember(context.Background(), "shared-modules", "azure")
	// Then
	if !errors.Is(err, ErrUnknownMember) {
		t.Fatalf("expected ErrUnknownMember, got %v", err)
	}
}

//nolint:paralleltest // embeddedGroupMembers is shared; concurrent Register calls aren't safe.
func TestManager_GroupMembers_ReturnsNamesRegisteredAtBuildTime(t *testing.T) {
	// Given
	const group = "infrastructure-presets"
	t.Cleanup(func() { delete(embeddedGroupMembers, group) })
	RegisterGroupMembers(group, []string{"aws", "azure"})
	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	members := manager.GroupMembers(group)
	// Then
	if !slices.Equal(members, []string{"aws", "azure"}) {
		t.Fatalf("expected the registered member names, got %v", members)
	}
}

func TestManager_GroupMembers_EmptyWhenNeverRegistered(t *testing.T) {
	t.Parallel()

	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")

	if members := manager.GroupMembers("never-registered"); len(members) != 0 {
		t.Fatalf("expected no members, got %v", members)
	}
}

// A Glob:true resource whose live source is a bare directory declares
// extract: false, since there is nothing to unpack to read it live. Once
// embedded, its bytes are always an archive, so resolving it from embedded
// data must still extract regardless of that declared value.
//
//nolint:paralleltest // embeddedResources/embeddedHashes are shared; concurrent Register isn't.
func TestManager_RequestMember_EmbeddedGlobResourceIsExtractedForMatching(t *testing.T) {
	// Given: a directory archived exactly as resourceembedder would for a
	// Glob:true, embedded resource — download_path names it as an archive so
	// Manager's extractor lookup recognizes the embedded blob.
	const resourceID = "embedded-glob-test"
	t.Cleanup(func() {
		delete(embeddedResources, resourceID)
		delete(embeddedHashes, resourceID)
	})
	archiveDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(
		t, archiveDir, "embedded-glob-test.tar.gz", map[string]tarFixtureEntry{
			"aws/main.tf": {Content: "aws module"},
		},
	)
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read fixture archive: %v", err)
	}
	Register(resourceID, data, sha256OfBytes(data))
	spec := ResourceSpec{
		resourceID: {
			Glob:  true,
			Embed: EmbedDefault,
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {
					URL:          "https://example.com/" + resourceID + ".tar.gz",
					Subpath:      "*",
					DownloadPath: resourceID + ".tar.gz",
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, t.TempDir(), "linux", "amd64")

	// When
	path, err := manager.RequestMember(context.Background(), resourceID, "aws")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "main.tf")); err != nil {
		t.Fatalf("expected main.tf inside the extracted match %q, got %v", path, err)
	}
}

func TestManager_RequestCopyWritesAPlainFile(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	server := newArtifactServer(t, "tool.bin", []byte("tool-content"))
	spec := ResourceSpec{
		"copy-file-test": {
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {
					URL:    server.URL + "/tool.bin",
					Sha256: sha256OfBytes([]byte("tool-content")),
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")
	destDir := filepath.Join(t.TempDir(), "nested", "destination")

	// When
	err := manager.RequestCopy(context.Background(), "copy-file-test", destDir)
	// Then
	if err != nil {
		t.Fatalf("expected materialization to succeed, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "tool.bin"))
	if err != nil {
		t.Fatalf("expected materialized file to exist, got %v", err)
	}
	if string(data) != "tool-content" {
		t.Fatalf("expected materialized content %q, got %q", "tool-content", data)
	}
}

func TestManager_RequestCopyExtractsAnArchive(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	archivePath := writeZipFixture(t, deploymentDir, "artifact.zip", map[string]string{
		"launcher": "runner-content",
	})
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read artifact fixture: %v", err)
	}
	server := newArtifactServer(t, "artifact.zip", archiveData)
	spec := ResourceSpec{
		"copy-archive-test": {
			Extract: true,
			Artifact: map[string]ArtifactSpec{
				anyPlatformKey: {
					URL:    server.URL + "/artifact.zip",
					Sha256: sha256OfTestFile(t, archivePath),
				},
			},
		},
	}
	manager := NewResourceManagerForPlatform(spec, deploymentDir, "linux", "amd64")
	destDir := filepath.Join(t.TempDir(), "destination")

	// When
	err = manager.RequestCopy(context.Background(), "copy-archive-test", destDir)
	// Then
	if err != nil {
		t.Fatalf("expected materialization to succeed, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "launcher"))
	if err != nil {
		t.Fatalf("expected extracted file to exist under destDir, got %v", err)
	}
	if string(data) != "runner-content" {
		t.Fatalf("expected extracted content %q, got %q", "runner-content", data)
	}
}

func TestManager_GetCopyMaterializesARuntimeConstructedDefinition(t *testing.T) {
	t.Parallel()

	// Given: an ad-hoc definition never registered in the manager's own spec,
	// the way an external preset path is resolved.
	srcDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(srcDir, "preset.yaml"),
		[]byte("content"),
		filePerm,
	); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	manager := NewResourceManagerForPlatform(ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{Artifact: map[string]ArtifactSpec{anyPlatformKey: {URL: srcDir}}}
	destDir := filepath.Join(t.TempDir(), "destination")

	// When
	err := manager.GetCopy(context.Background(), def, "external-preset", destDir)
	// Then
	if err != nil {
		t.Fatalf("expected materialization to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "preset.yaml")); err != nil {
		t.Fatalf("expected copied file to exist, got %v", err)
	}
}
