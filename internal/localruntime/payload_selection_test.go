// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	localassets "github.com/exasol/exasol-personal/internal/localruntime/assets"
)

//nolint:paralleltest // mutates package-level payload selection hooks.
func TestRuntimeEnsurePayloadSelected_PersistsRunAndBootAssetPaths(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	cacheDir := t.TempDir()
	runBytes := []byte("run")
	kernelBytes := []byte("kernel")
	initrdBytes := []byte("initrd")
	architecture := localPayloadArchitecture()
	bundleBytes := mustBuildPayloadBundle(t, map[string]string{
		"bundle/exasol-nano-db.run":    string(runBytes),
		"bundle/vmlinux.container":     string(kernelBytes),
		"bundle/ubuntu-initrd.cpio.gz": string(initrdBytes),
	})

	originalDefaultCacheDir := defaultPayloadCacheDir
	originalResolveEmbeddedPayload := resolveEmbeddedPayload
	originalSeedEmbeddedPayload := seedEmbeddedPayload
	t.Cleanup(func() {
		defaultPayloadCacheDir = originalDefaultCacheDir
		resolveEmbeddedPayload = originalResolveEmbeddedPayload
		seedEmbeddedPayload = originalSeedEmbeddedPayload
	})

	defaultPayloadCacheDir = func() (string, error) {
		return cacheDir, nil
	}
	resolveEmbeddedPayload = func(expectedArchitecture string) (*localassets.EmbeddedPayload, error) {
		return localassets.LoadEmbeddedPayload(
			[]byte(`{
			  "version": "1.2.3",
			  "architecture": "`+architecture+`",
			  "run": {"filename": "exasol-nano-db.run", "sha256": "`+sha256Hex(runBytes)+`"},
			  "boot": {
			    "kernel": {"filename": "vmlinux.container", "sha256": "`+sha256Hex(kernelBytes)+`"},
			    "initrd": {"filename": "ubuntu-initrd.cpio.gz", "sha256": "`+sha256Hex(initrdBytes)+`"}
			  }
			}`),
			bundleBytes,
			expectedArchitecture,
		)
	}
	seedEmbeddedPayload = localassets.SeedEmbeddedPayload

	// When
	payload, err := runtime.EnsurePayloadSelected(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected payload selection to succeed, got %v", err)
	}
	if payload.Boot == nil {
		t.Fatal("expected boot assets to be persisted")
	}
	if !isCachedFile(payload.CachePath) {
		t.Fatalf("expected cached run payload path, got %q", payload.CachePath)
	}
	if !isCachedFile(payload.Boot.KernelPath) {
		t.Fatalf("expected cached kernel path, got %q", payload.Boot.KernelPath)
	}
	if !isCachedFile(payload.Boot.InitrdPath) {
		t.Fatalf("expected cached initrd path, got %q", payload.Boot.InitrdPath)
	}
}

//nolint:paralleltest // mutates package-level payload selection hooks.
func TestRuntimeEnsurePayloadSelected_RejectsMissingBootMetadata(t *testing.T) {
	// Given
	runtime := New(t.TempDir())
	architecture := localPayloadArchitecture()
	bundleBytes := mustBuildPayloadBundle(t, map[string]string{
		"bundle/exasol-nano-db.run": "run",
	})

	originalDefaultCacheDir := defaultPayloadCacheDir
	originalResolveEmbeddedPayload := resolveEmbeddedPayload
	t.Cleanup(func() {
		defaultPayloadCacheDir = originalDefaultCacheDir
		resolveEmbeddedPayload = originalResolveEmbeddedPayload
	})

	defaultPayloadCacheDir = func() (string, error) {
		return t.TempDir(), nil
	}
	resolveEmbeddedPayload = func(expectedArchitecture string) (*localassets.EmbeddedPayload, error) {
		return localassets.LoadEmbeddedPayload(
			[]byte(`{
			  "version": "1.2.3",
			  "architecture": "`+architecture+`",
			  "run": {"filename": "exasol-nano-db.run", "sha256": "`+sha256Hex([]byte("run"))+`"}
			}`),
			bundleBytes,
			expectedArchitecture,
		)
	}

	// When
	_, err := runtime.EnsurePayloadSelected(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing boot metadata to fail")
	}
	if err.Error() == "" {
		t.Fatal("expected a descriptive error")
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mustBuildPayloadBundle(t *testing.T, files map[string]string) []byte {
	t.Helper()

	path := t.TempDir() + "/payload.tar.gz"
	if err := writeTarGzArchive(path, files); err != nil {
		t.Fatalf("expected payload bundle fixture, got %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected payload bundle fixture to be readable, got %v", err)
	}

	return data
}

func writeTarGzArchive(path string, files map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}
