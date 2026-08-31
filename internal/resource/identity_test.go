// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
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

	index, _, err := manager.cache.readIndex()
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
