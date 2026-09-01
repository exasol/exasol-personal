// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

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

func TestResolverListGroupMembersWithoutMaterializingThem(t *testing.T) {
	t.Parallel()

	// Given
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter, _ *http.Request,
	) {
		requests.Add(1)
		_, _ = writer.Write([]byte("payload"))
	}))
	defer server.Close()
	resolver := newTestResolverForPlatform(t, ResourceSpec{
		"presets/aws":   testResourceDefinition(server.URL+"/aws", "aws"),
		"presets/azure": testResourceDefinition(server.URL+"/azure", "azure"),
		"other":         testResourceDefinition(server.URL+"/other", "other"),
	}, t.TempDir(), "linux", "amd64")

	// When
	members := resolver.List("presets/")

	// Then
	if !slices.Equal(members, []string{"presets/aws", "presets/azure"}) {
		t.Fatalf("members = %v", members)
	}
	if requests.Load() != 0 {
		t.Fatalf("listing transferred %d resources", requests.Load())
	}
}

func TestResolverListUnknownGroupReturnsNoMembers(t *testing.T) {
	t.Parallel()

	// Given
	resolver := newTestResolverForPlatform(t, ResourceSpec{
		"presets/aws": testResourceDefinition("https://example.com/aws", "aws"),
	}, t.TempDir(), "linux", "amd64")

	// When
	members := resolver.List("unknown/")

	// Then
	if len(members) != 0 {
		t.Fatalf("members = %v, want none", members)
	}
}

func TestResolverResolveGroupMemberLeavesSiblingUnmaterialized(t *testing.T) {
	t.Parallel()

	// Given
	var awsRequests, azureRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter, request *http.Request,
	) {
		if request.URL.Path == "/aws" {
			awsRequests.Add(1)
		} else {
			azureRequests.Add(1)
		}
		content := "azure"
		if request.URL.Path == "/aws" {
			content = "aws"
		}
		_, _ = writer.Write([]byte(content))
	}))
	defer server.Close()
	resolver := newTestResolverForPlatform(t, ResourceSpec{
		"presets/aws":   testResourceDefinition(server.URL+"/aws", "aws"),
		"presets/azure": testResourceDefinition(server.URL+"/azure", "azure"),
	}, t.TempDir(), "linux", "amd64")

	// When
	_, err := resolver.Resolve(context.Background(), "presets/aws")
	// Then
	if err != nil {
		t.Fatalf("resolve member: %v", err)
	}
	if awsRequests.Load() != 1 || azureRequests.Load() != 0 {
		t.Fatalf("requests: aws=%d azure=%d", awsRequests.Load(), azureRequests.Load())
	}
}

func TestResolverUnknownGroupMemberErrorNamesMemberAndGroup(t *testing.T) {
	t.Parallel()

	// Given
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	_, err := resolver.Resolve(context.Background(), "presets/missing")

	// Then
	if err == nil || !strings.Contains(err.Error(), "presets") ||
		!strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v, want group and member names", err)
	}
}

func testResourceDefinition(url, content string) ResourceDefinition {
	return ResourceDefinition{Artifact: map[string]ArtifactSpec{
		"any": {
			URL:          url,
			Sha256:       sha256OfBytes([]byte(content)),
			DownloadPath: "payload.bin",
		},
	}}
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

func newTestResolution(
	t *testing.T,
	resolver *Resolver,
	def ResourceDefinition,
) (resolution, error) {
	t.Helper()

	return resolver.newResolution(def, def.Artifact[anyPlatformKey], Probe{}, "sha256:test")
}

// A subpath selects within resolved content whatever the source kind, so it
// applies to a plain download just as it does to an extracted archive.
func TestResolver_Resolution_AppliesSubpathWithoutExtraction(t *testing.T) {
	t.Parallel()

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "https://example.com/repo.git", Subpath: "infra/aws"},
		},
	}

	res, err := newTestResolution(t, resolver, def)
	if err != nil {
		t.Fatalf("expected a resolution, got %v", err)
	}
	resolved, err := resolver.resolvedPath(res)
	if err != nil {
		t.Fatalf("expected a resolved path, got %v", err)
	}

	wantSuffix := filepath.Join("repo.git", "infra", "aws")
	if !strings.HasSuffix(resolved, wantSuffix) {
		t.Fatalf("expected resolved path to end with %q, got %q", wantSuffix, resolved)
	}
}

func TestResolver_Resolution_RejectsTraversalSubpath(t *testing.T) {
	t.Parallel()

	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")
	def := ResourceDefinition{
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "https://example.com/repo.git", Subpath: "../escape"},
		},
	}

	_, err := newTestResolution(t, resolver, def)
	if err == nil || !strings.Contains(err.Error(), "subpath") {
		t.Fatalf("expected subpath traversal error, got %v", err)
	}
}

// A subpath is presentation, not identity, so selecting two subpaths of one
// source stores the source once instead of fetching it twice.
func TestResolver_Resolution_SubpathsShareOneCacheEntry(t *testing.T) {
	t.Parallel()

	// Given
	fixtureDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(
		t,
		fixtureDir,
		"presets.tar.gz",
		map[string]tarFixtureEntry{
			"infra/aws/manifest.yaml":   {Content: "aws", Mode: 0o600},
			"infra/azure/manifest.yaml": {Content: "azure", Mode: 0o600},
		},
	)
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	server, downloads := newCountingArtifactServer(t, "presets.tar.gz", archiveData)
	definition := func(subpath string) ResourceDefinition {
		return ResourceDefinition{
			Extract: true,
			Artifact: map[string]ArtifactSpec{anyPlatformKey: {
				URL:          server.URL + "/presets.tar.gz",
				Sha256:       sha256OfBytes(archiveData),
				DownloadPath: "presets.tar.gz",
				Subpath:      subpath,
			}},
		}
	}
	resolver := newTestResolverForPlatform(t, ResourceSpec{
		"aws":   definition("infra/aws/manifest.yaml"),
		"azure": definition("infra/azure/manifest.yaml"),
	}, t.TempDir(), "linux", "amd64")

	// When
	awsPath, err := resolver.Resolve(context.Background(), "aws")
	if err != nil {
		t.Fatalf("resolve aws: %v", err)
	}
	azurePath, err := resolver.Resolve(context.Background(), "azure")
	if err != nil {
		t.Fatalf("resolve azure: %v", err)
	}

	// Then
	awsContent, err := os.ReadFile(awsPath)
	if err != nil {
		t.Fatalf("read aws preset: %v", err)
	}
	azureContent, err := os.ReadFile(azurePath)
	if err != nil {
		t.Fatalf("read azure preset: %v", err)
	}
	if string(awsContent) != "aws" || string(azureContent) != "azure" {
		t.Fatalf("resolved content: aws=%q azure=%q", awsContent, azureContent)
	}
	if downloads.Load() != 1 {
		t.Fatalf("expected one download, got %d", downloads.Load())
	}
}

// Extraction is presentation too, so an extracted and an unextracted view of
// one source share the download.
func TestResolver_Resolution_ExtractedAndPlainShareOneCacheEntry(t *testing.T) {
	t.Parallel()

	// Given
	fixtureDir := t.TempDir()
	archivePath := writeTarGzMultiFixture(
		t,
		fixtureDir,
		"tool.tar.gz",
		map[string]tarFixtureEntry{"tool": {Content: "payload", Mode: 0o600}},
	)
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	server, downloads := newCountingArtifactServer(t, "tool.tar.gz", archiveData)
	definition := func(extract bool) ResourceDefinition {
		return ResourceDefinition{
			Extract: extract,
			Artifact: map[string]ArtifactSpec{anyPlatformKey: {
				URL:          server.URL + "/tool.tar.gz",
				Sha256:       sha256OfBytes(archiveData),
				DownloadPath: "tool.tar.gz",
			}},
		}
	}
	resolver := newTestResolverForPlatform(t, ResourceSpec{
		"plain":     definition(false),
		"extracted": definition(true),
	}, t.TempDir(), "linux", "amd64")

	// When
	plainPath, err := resolver.Resolve(context.Background(), "plain")
	if err != nil {
		t.Fatalf("resolve plain archive: %v", err)
	}
	extractedPath, err := resolver.Resolve(context.Background(), "extracted")
	if err != nil {
		t.Fatalf("resolve extracted archive: %v", err)
	}

	// Then
	plainContent, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatalf("read plain archive: %v", err)
	}
	if !slices.Equal(plainContent, archiveData) {
		t.Fatal("plain view differs from the downloaded archive")
	}
	extractedContent, err := os.ReadFile(filepath.Join(extractedPath, "tool"))
	if err != nil {
		t.Fatalf("read extracted tool: %v", err)
	}
	if string(extractedContent) != "payload" {
		t.Fatalf("extracted content = %q, want payload", extractedContent)
	}
	if downloads.Load() != 1 {
		t.Fatalf("expected one download, got %d", downloads.Load())
	}
}

func TestResolver_RequestUsesDownloadPathFallback(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, spec, deploymentDir, "linux", "amd64")

	// When
	path, err := resolver.Resolve(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertPathInCache(t, deploymentDir, path, "artifact.tar.gz")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected artifact path to exist, got %v", err)
	}
}

func TestResolver_RequestRejectsDownloadPathEscape(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, spec, deploymentDir, "linux", "amd64")

	// When
	_, err = resolver.Resolve(context.Background(), "artifact")
	// Then
	if err == nil || !strings.Contains(err.Error(), "must stay within") {
		t.Fatalf("expected resource-dir containment error, got %v", err)
	}
}

func TestResolver_RequestUsesPlatformVariantAndCachesIt(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, spec, deploymentDir, "linux", "amd64")

	// When
	path1, err := resolver.Resolve(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path1 == "" {
		t.Fatal("expected resolved path")
	}
	assertPathInCache(t, deploymentDir, path1, filepath.Join("unpack", "tool"))
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
	index, err := resolver.cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read cache index: %v", err)
	}
	entryID, entry := onlyCacheEntry(t, index)
	expectedSize, err := directorySize(resolver.cache.entryDir(entryID))
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
	path2, err := resolver.Resolve(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected no error on cache hit, got %v", err)
	}
	if path2 != path1 {
		t.Fatalf("expected cache hit to return same path, got %q and %q", path1, path2)
	}
}

func TestResolver_RequestUpdatesLastUsedAtOnCacheHit(t *testing.T) {
	t.Parallel()

	// Given
	clock := &testClock{now: testNow()}
	cache := newCacheWithClock(
		filepath.Join(t.TempDir(), "cache"),
		filepath.Join(t.TempDir(), cacheConfigFileName),
		clock.Now,
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
	resolver := newTestResolverWithCacheForPlatform(t, spec, cache, "linux", "amd64")

	// When
	path1, err := resolver.Resolve(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	index, err := cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read cache index: %v", err)
	}
	_, entry := onlyCacheEntry(t, index)
	if !entry.CreatedAt.Equal(clock.now) || !entry.LastUsedAt.Equal(clock.now) {
		t.Fatalf("unexpected initial timestamps: %+v", entry)
	}

	// When
	clock.now = clock.now.Add(2 * time.Hour)
	path2, err := resolver.Resolve(context.Background(), "artifact")
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
	index, err = cache.readIndex()
	if err != nil {
		t.Fatalf("failed to read cache index: %v", err)
	}
	_, entry = onlyCacheEntry(t, index)
	if !entry.CreatedAt.Equal(testNow()) {
		t.Fatalf("expected created timestamp to remain unchanged, got %s", entry.CreatedAt)
	}
	if !entry.LastUsedAt.Equal(clock.now) {
		t.Fatalf("expected last-used timestamp %s, got %s", clock.now, entry.LastUsedAt)
	}
}

func TestResolver_RequestRefreshesMissingCachedFile(t *testing.T) {
	t.Parallel()

	// Given
	clock := &testClock{now: testNow()}
	cache := newCacheWithClock(
		filepath.Join(t.TempDir(), "cache"),
		filepath.Join(t.TempDir(), cacheConfigFileName),
		clock.Now,
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
	resolver := newTestResolverWithCacheForPlatform(t, spec, cache, "linux", "amd64")
	path1, err := resolver.Resolve(context.Background(), "artifact")
	if err != nil {
		t.Fatalf("expected first request to succeed, got %v", err)
	}
	if err := os.Remove(path1); err != nil {
		t.Fatalf("failed to remove cached artifact: %v", err)
	}

	// When
	clock.now = clock.now.Add(time.Hour)
	path2, err := resolver.Resolve(context.Background(), "artifact")
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

func TestResolver_RequestRunsAutomaticStaleCleanupWhenDue(t *testing.T) {
	t.Parallel()

	// Given
	clock := &testClock{now: testNow()}
	cache := newCacheWithClock(
		filepath.Join(t.TempDir(), "cache"),
		filepath.Join(t.TempDir(), cacheConfigFileName),
		clock.Now,
	)
	writeTestCacheConfig(t, cache, 1)
	index := emptyCacheIndex()
	seedCacheEntry(
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
	resolver := newTestResolverWithCacheForPlatform(t, spec, cache, "linux", "amd64")

	// When
	_, err := resolver.Resolve(context.Background(), "artifact")
	// Then
	if err != nil {
		t.Fatalf("expected request to succeed, got %v", err)
	}
	if _, err := os.Stat(cache.entryDir("stale")); !os.IsNotExist(err) {
		t.Fatalf("expected stale entry to be removed, got %v", err)
	}
	read, err := cache.readIndex()
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

func TestResolver_RequestReportsChecksumMismatch(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, spec, deploymentDir, "linux", "amd64")

	// When
	_, err = resolver.Resolve(context.Background(), "artifact")
	// Then
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "got") {
		t.Fatalf("expected checksum details in error, got %v", err)
	}
}

func TestResolver_RequestRefreshesWhenChecksumChanges(t *testing.T) {
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
	resolverV1 := newTestResolverForPlatform(t, specV1, deploymentDir, "linux", "amd64")

	// When
	path1, err := resolverV1.Resolve(context.Background(), "artifact")
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
	resolverV2 := newTestResolverForPlatform(t, specV2, deploymentDir, "linux", "amd64")

	// When
	path2, err := resolverV2.Resolve(context.Background(), "artifact")
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

// Entries live in one flat directory named by the cache key, so a cached path
// is identified by its root and its tail, not by a resource or platform
// segment in the middle.
func assertPathInCache(t *testing.T, cacheRoot, actualPath, suffix string) {
	t.Helper()

	prefix := filepath.Join(cacheRoot, artifactsDirName)
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

func onlyCacheEntry(t *testing.T, index cacheIndex) (string, cacheIndexEntry) {
	t.Helper()

	if len(index.Entries) != 1 {
		t.Fatalf("expected one cache entry, got %+v", index.Entries)
	}
	for key, entry := range index.Entries {
		return key, entry
	}

	t.Fatal("expected one cache entry")

	return "", cacheIndexEntry{}
}

func TestResolver_GetWithRuntimeDefinition(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, deploymentDir, "linux", "amd64")

	// When
	path, err := resolveTestDefinition(context.Background(), resolver, def, "tool-binary")
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

func TestResolver_GetNoChecksumAlwaysRefetches(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, deploymentDir, "linux", "amd64")

	// When
	_, err := resolveTestDefinition(context.Background(), resolver, def, "tool-binary")
	if err != nil {
		t.Fatalf("expected first Get to succeed, got %v", err)
	}
	_, err = resolveTestDefinition(context.Background(), resolver, def, "tool-binary")
	if err != nil {
		t.Fatalf("expected second Get to succeed, got %v", err)
	}
	// Then
	if requests.Load() != 2 {
		t.Fatalf("expected 2 downloads for no-checksum archive, got %d", requests.Load())
	}
}

func TestResolver_GetGitSourceCachedOnSameCommit(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")

	// When — first fetch clones the repo
	path, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}

	// Corrupt a file in the cache to detect whether Fetch is called again.
	corruptedFile := filepath.Join(path, "preset.txt")
	if err := os.WriteFile(corruptedFile, []byte("corrupted"), filePerm); err != nil {
		t.Fatalf("corrupt failed: %v", err)
	}

	// When — second Get with same commit; Identify returns same hash → cache hit
	path2, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
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

func TestResolver_GetGitSourceRefetchesOnNewCommit(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")

	_, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}

	// Advance the remote
	addCommitToTestRepo(t, repoDir, "v2.txt", "v2")

	// When — second Get with new commit
	path, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}

	// Then — new content is present
	if _, err := os.Stat(filepath.Join(path, "v2.txt")); err != nil {
		t.Fatalf("expected v2.txt after re-fetch, got %v", err)
	}
}

func TestResolver_GetFileDirectoryReturnedDirectly(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := resolveTestDefinition(context.Background(), resolver, def, "preset-dir")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Fetch returns a redirect path for local directories; the Resolver records it
	// in the cache index but does not write an artifact to the cache directory.
	if strings.HasPrefix(path, cacheDir) {
		t.Fatalf("expected original path, not a cache path; got %q", path)
	}
	if path != presetDir {
		t.Fatalf("expected path %q, got %q", presetDir, path)
	}
}

func TestResolver_GetFileDirectoryWithResourcePathReturnsSubdirectory(t *testing.T) {
	t.Parallel()

	// Given — a preset root with a nested subdirectory. The resolver must apply
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	path, err := resolveTestDefinition(context.Background(), resolver, def, "preset-dir")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != subDir {
		t.Fatalf("expected path %q, got %q", subDir, path)
	}
}

func TestResolver_GetFileDirectoryRejectsTraversalResourcePath(t *testing.T) {
	t.Parallel()

	// Given — a subpath that tries to escape the redirect root.
	presetDir := t.TempDir()
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file://" + presetDir, Subpath: "../escape"},
		},
	}
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	_, err := resolveTestDefinition(context.Background(), resolver, def, "preset-dir")
	// Then
	if err == nil || !strings.Contains(err.Error(), "subpath") {
		t.Fatalf("expected subpath traversal error, got %v", err)
	}
}

func TestResolver_GetFileBareFileReturnedDirectly(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := resolveTestDefinition(context.Background(), resolver, def, "local-launcher")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != binaryPath {
		t.Fatalf("expected path %q, got %q", binaryPath, path)
	}
}

func TestResolver_GetFileDirectoryMissingReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	def := ResourceDefinition{
		Extract: false,
		Artifact: map[string]ArtifactSpec{
			anyPlatformKey: {URL: "file:///nonexistent/path/to/preset"},
		},
	}
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, t.TempDir(), "linux", "amd64")

	// When
	_, err := resolveTestDefinition(context.Background(), resolver, def, "preset-missing")
	// Then
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does-not-exist error, got %v", err)
	}
}

func TestResolver_GetFileArchiveExtractedIntoCache(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
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

func TestResolver_GetAnyPlatformFallback(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, spec, deploymentDir, "darwin", "arm64")

	// When
	path, err := resolver.Resolve(context.Background(), "tool")
	// Then
	if err != nil {
		t.Fatalf("expected any-platform fallback to succeed, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected resolved path to exist, got %v", err)
	}
}

func TestResolver_GetPlatformSpecificTakesPriorityOverAny(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, spec, deploymentDir, "linux", "amd64")

	// When
	path, err := resolver.Resolve(context.Background(), "tool")
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

func TestResolver_GetZipExtraction(t *testing.T) {
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
	resolver := newTestResolverForPlatform(t, ResourceSpec{}, cacheDir, "linux", "amd64")

	// When
	path, err := resolveTestDefinition(context.Background(), resolver, def, "preset")
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
