// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func embeddedResolver(t *testing.T, spec string, blobs fstest.MapFS) *Resolver {
	t.Helper()

	resolver, err := New(Options{
		Spec:      []byte(spec),
		Blobs:     blobs,
		CacheRoot: t.TempDir(),
		Platform:  Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err != nil {
		t.Fatalf("failed to build a resolver: %v", err)
	}

	return resolver
}

func TestEmbeddedSource_ResolvesWithoutNetworkAccess(t *testing.T) {
	t.Parallel()

	payload := []byte("embedded payload")
	resolver := embeddedResolver(t, `
tool:
  artifact:
    any:
      url: embedded://blobs/tool.bin
      sha256: `+sha256OfBytes(payload)+`
      download_path: tool.bin
`, fstest.MapFS{"blobs/tool.bin": {Data: payload}})

	path, err := resolver.Resolve(context.Background(), "tool")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read resolved artifact: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("resolved content = %q, want %q", got, payload)
	}
}

func TestEmbeddedSource_MissingDataIsAHardFailure(t *testing.T) {
	t.Parallel()

	resolver := embeddedResolver(t, `
tool:
  artifact:
    any:
      url: embedded://blobs/absent.bin
      sha256: `+strings.Repeat("a", 64)+`
      download_path: absent.bin
`, fstest.MapFS{})

	_, err := resolver.Resolve(context.Background(), "tool")
	if err == nil {
		t.Fatal("expected resolution to fail")
	}
	if !strings.Contains(err.Error(), "no embedded data") {
		t.Fatalf("expected a missing-data error, got %v", err)
	}
}

func TestEmbeddedSource_OtherSourcesNeverConsultEmbeddedData(t *testing.T) {
	t.Parallel()

	resolver := embeddedResolver(t, `
tool:
  artifact:
    any:
      url: file:///nonexistent/tool.bin
`, fstest.MapFS{"blobs/tool.bin": {Data: []byte("embedded payload")}})

	_, err := resolver.Resolve(context.Background(), "tool")
	if err == nil {
		t.Fatal("expected resolution to fail rather than use embedded data")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected the local source's own error, got %v", err)
	}
}

func TestEmbeddedSource_DifferentContentResolvesToDistinctEntries(t *testing.T) {
	t.Parallel()

	cacheRoot := t.TempDir()
	resolve := func(payload []byte) string {
		t.Helper()

		resolver, err := New(Options{
			Spec: []byte(`
tool:
  artifact:
    any:
      url: embedded://blobs/tool.bin
      sha256: ` + sha256OfBytes(payload) + `
      download_path: tool.bin
`),
			Blobs:     fstest.MapFS{"blobs/tool.bin": {Data: payload}},
			CacheRoot: cacheRoot,
			Platform:  Platform{GOOS: "linux", GOARCH: "amd64"},
		})
		if err != nil {
			t.Fatalf("failed to build a resolver: %v", err)
		}
		path, err := resolver.Resolve(context.Background(), "tool")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}

		return path
	}

	first := resolve([]byte("first build"))
	second := resolve([]byte("second build"))

	if first == second {
		t.Fatalf("expected distinct cache entries, both were %q", first)
	}
	for path, want := range map[string]string{first: "first build", second: "second build"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %q: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("entry %q holds %q, want %q", path, got, want)
		}
	}
}

func TestEmbeddedSource_ArchiveIsExtracted(t *testing.T) {
	t.Parallel()

	// Given
	archive := tarGzBytes(t, map[string]string{"launcher": "#!/bin/sh\n"})
	resolver := embeddedResolver(t, `
runner:
  extract: true
  artifact:
    any:
      url: embedded://blobs/runner.tar.gz
      sha256: `+sha256OfBytes(archive)+`
      download_path: runner.tar.gz
`, fstest.MapFS{"blobs/runner.tar.gz": {Data: archive}})

	// When
	path, err := resolver.Resolve(context.Background(), "runner")
	// Then
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	launcher := filepath.Join(path, "launcher")
	if _, err := os.Stat(launcher); err != nil {
		t.Fatalf("expected the archive to be extracted, got %v", err)
	}
	if err := os.WriteFile(launcher, []byte("preserved"), filePerm); err != nil {
		t.Fatalf("modify extracted file: %v", err)
	}

	// When
	reusedPath, err := resolver.Resolve(context.Background(), "runner")
	// Then
	if err != nil {
		t.Fatalf("resolve again: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(reusedPath, "launcher"))
	if err != nil {
		t.Fatalf("read reused extraction: %v", err)
	}
	if reusedPath != path || string(content) != "preserved" {
		t.Fatalf("extraction was recreated: path=%q content=%q", reusedPath, content)
	}
}

func TestEmbeddedSource_AbsentBlobsLeaveOtherSourcesWorking(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), filePerm); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolver, err := New(Options{
		Spec:      []byte("preset:\n  artifact:\n    any:\n      url: file://" + dir + "\n"),
		CacheRoot: t.TempDir(),
		Platform:  Platform{GOOS: "linux", GOARCH: "amd64"},
	})
	if err != nil {
		t.Fatalf("failed to build a resolver: %v", err)
	}

	path, err := resolver.Resolve(context.Background(), "preset")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if path != dir {
		t.Fatalf("resolved to %q, want %q", path, dir)
	}
}

func tarGzBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	dir := t.TempDir()
	for name, content := range entries {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), filePerm); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range entries {
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write body %s: %v", name, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	return buf.Bytes()
}
