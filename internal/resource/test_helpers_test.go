// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeZipFixtureEntries(
	t *testing.T,
	dir, archiveName string,
	entries map[string]string,
) string {
	t.Helper()

	archivePath := filepath.Join(dir, archiveName)
	zipFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create zip fixture: %v", err)
	}
	zipWriter := zip.NewWriter(zipFile)

	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, entryName := range keys {
		fw, err := zipWriter.Create(entryName)
		if err != nil {
			_ = zipFile.Close()
			t.Fatalf("failed to create zip entry %q: %v", entryName, err)
		}
		if _, err := fw.Write([]byte(entries[entryName])); err != nil {
			_ = zipFile.Close()
			t.Fatalf("failed to write zip entry %q: %v", entryName, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		_ = zipFile.Close()
		t.Fatalf("failed to close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("failed to close zip file: %v", err)
	}

	return archivePath
}

func newTestResolverForPlatform(
	t *testing.T,
	spec ResourceSpec,
	cacheRoot, goos, goarch string,
) *Resolver {
	t.Helper()

	resolver, err := New(Options{
		Definitions: spec,
		CacheRoot:   cacheRoot,
		Platform:    Platform{GOOS: goos, GOARCH: goarch},
	})
	if err != nil {
		t.Fatalf("create resource resolver: %v", err)
	}

	return resolver
}

func newTestResolverWithCacheForPlatform(
	t *testing.T,
	spec ResourceSpec,
	cache *Cache,
	goos, goarch string,
) *Resolver {
	t.Helper()

	resolver, err := New(Options{
		Definitions: spec,
		Cache:       cache,
		Platform:    Platform{GOOS: goos, GOARCH: goarch},
	})
	if err != nil {
		t.Fatalf("create resource resolver: %v", err)
	}

	return resolver
}

func resolveTestDefinition(
	ctx context.Context,
	resolver *Resolver,
	definition ResourceDefinition,
	resourceID string,
) (string, error) {
	testResolver := *resolver
	testResolver.spec = ResourceSpec{resourceID: definition}

	return testResolver.Resolve(ctx, resourceID)
}
