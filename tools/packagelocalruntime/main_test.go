// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	localassets "github.com/exasol/exasol-personal/internal/localruntime/assets"
)

func TestBuildMetadataUsesInputFilenamesAndChecksums(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	runPath := writeTempFile(t, tempDir, "payload.run", "run payload")
	kernelPath := writeTempFile(t, tempDir, "vmlinux.container", "kernel payload")
	initrdPath := writeTempFile(t, tempDir, "ubuntu-initrd.cpio.gz", "initrd payload")

	metadata, err := buildMetadata("2026.1.0", "arm64", runPath, kernelPath, initrdPath)
	if err != nil {
		t.Fatalf("expected metadata build to succeed, got %v", err)
	}

	if metadata.Version != "2026.1.0" {
		t.Fatalf("expected version %q, got %q", "2026.1.0", metadata.Version)
	}
	if metadata.Architecture != "arm64" {
		t.Fatalf("expected architecture %q, got %q", "arm64", metadata.Architecture)
	}
	if metadata.Run == nil || metadata.Run.Filename != "payload.run" || metadata.Run.SHA256 == "" {
		t.Fatalf("unexpected run metadata: %#v", metadata.Run)
	}
	if metadata.Boot == nil || metadata.Boot.Kernel == nil || metadata.Boot.Initrd == nil {
		t.Fatalf("unexpected boot metadata: %#v", metadata.Boot)
	}
	if metadata.Boot.Kernel.Filename != "vmlinux.container" || metadata.Boot.Kernel.SHA256 == "" {
		t.Fatalf("unexpected kernel metadata: %#v", metadata.Boot.Kernel)
	}
	if metadata.Boot.Initrd.Filename != "ubuntu-initrd.cpio.gz" || metadata.Boot.Initrd.SHA256 == "" {
		t.Fatalf("unexpected initrd metadata: %#v", metadata.Boot.Initrd)
	}
}

func TestWriteBundleArchiveIncludesRunKernelAndInitrd(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	runPath := writeTempFile(t, tempDir, "payload.run", "run payload")
	kernelPath := writeTempFile(t, tempDir, "vmlinux.container", "kernel payload")
	initrdPath := writeTempFile(t, tempDir, "ubuntu-initrd.cpio.gz", "initrd payload")
	archivePath := filepath.Join(tempDir, "payload.tar.gz")
	destinationRoot := filepath.Join(tempDir, "extracted")

	if err := writeBundleArchive(archivePath, map[string]string{
		"run":    runPath,
		"kernel": kernelPath,
		"initrd": initrdPath,
	}); err != nil {
		t.Fatalf("expected archive build to succeed, got %v", err)
	}

	bundle, err := localassets.PrepareBundle(archivePath, destinationRoot)
	if err != nil {
		t.Fatalf("expected archive extraction to succeed, got %v", err)
	}
	if filepath.Base(bundle.RunPath) != "payload.run" {
		t.Fatalf("expected run file %q, got %q", "payload.run", filepath.Base(bundle.RunPath))
	}
	if filepath.Base(bundle.KernelPath) != "vmlinux.container" {
		t.Fatalf("expected kernel file %q, got %q", "vmlinux.container", filepath.Base(bundle.KernelPath))
	}
	if filepath.Base(bundle.InitrdPath) != "ubuntu-initrd.cpio.gz" {
		t.Fatalf("expected initrd file %q, got %q", "ubuntu-initrd.cpio.gz", filepath.Base(bundle.InitrdPath))
	}
}

func writeTempFile(t *testing.T, dir string, name string, contents string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write test file %q: %v", name, err)
	}

	return path
}
