// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareBundle_UsesExistingDirectoryLayout(t *testing.T) {
	t.Parallel()

	// Given
	sourceDir := t.TempDir()
	kernelPath := filepath.Join(sourceDir, "payload", "vmlinux.container")
	initrdPath := filepath.Join(sourceDir, "payload", "ubuntu-initrd.cpio.gz")
	if err := os.MkdirAll(filepath.Dir(kernelPath), 0o700); err != nil {
		t.Fatalf("expected kernel dir to be created, got %v", err)
	}
	if err := os.WriteFile(kernelPath, []byte("kernel"), 0o600); err != nil {
		t.Fatalf("expected kernel fixture to be written, got %v", err)
	}
	if err := os.WriteFile(initrdPath, []byte("initrd"), 0o600); err != nil {
		t.Fatalf("expected initrd fixture to be written, got %v", err)
	}

	// When
	bundle, err := PrepareBundle(sourceDir, filepath.Join(t.TempDir(), "ignored"))

	// Then
	if err != nil {
		t.Fatalf("expected directory bundle preparation to succeed, got %v", err)
	}
	if bundle.KernelPath != kernelPath {
		t.Fatalf("expected kernel path %q, got %q", kernelPath, bundle.KernelPath)
	}
	if bundle.InitrdPath != initrdPath {
		t.Fatalf("expected initrd path %q, got %q", initrdPath, bundle.InitrdPath)
	}
}

func TestPrepareBundle_ExtractsTarGzArchive(t *testing.T) {
	t.Parallel()

	// Given
	archivePath := filepath.Join(t.TempDir(), "payload.tar.gz")
	if err := writeTarGzArchive(archivePath, map[string]string{
		"bundle/vmlinux.container":     "kernel",
		"bundle/ubuntu-initrd.cpio.gz": "initrd",
	}); err != nil {
		t.Fatalf("expected archive fixture to be written, got %v", err)
	}
	destinationRoot := filepath.Join(t.TempDir(), "bundle")

	// When
	bundle, err := PrepareBundle(archivePath, destinationRoot)

	// Then
	if err != nil {
		t.Fatalf("expected archive bundle preparation to succeed, got %v", err)
	}
	if filepath.Dir(bundle.KernelPath) != filepath.Join(destinationRoot, "bundle") {
		t.Fatalf("expected extracted kernel under %q, got %q", destinationRoot, bundle.KernelPath)
	}
	if filepath.Base(bundle.InitrdPath) != "ubuntu-initrd.cpio.gz" {
		t.Fatalf("expected initrd filename to be preserved, got %q", bundle.InitrdPath)
	}
}

func TestPrepareBundle_RejectsMissingRequiredFiles(t *testing.T) {
	t.Parallel()

	// Given
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "vmlinux.container"), []byte("kernel"), 0o600); err != nil {
		t.Fatalf("expected kernel fixture to be written, got %v", err)
	}

	// When
	_, err := PrepareBundle(sourceDir, filepath.Join(t.TempDir(), "ignored"))

	// Then
	if err == nil {
		t.Fatal("expected missing initrd to fail")
	}
	if !errors.Is(err, ErrPayloadBundleInvalid) {
		t.Fatalf("expected payload bundle invalid error, got %v", err)
	}
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
