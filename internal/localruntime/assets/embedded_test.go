// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmbeddedPayload_ValidatesMetadataAndArchitecture(t *testing.T) {
	t.Parallel()

	runBytes := []byte("runfile")
	kernelBytes := []byte("kernel")
	initrdBytes := []byte("initrd")

	bundleBytes, err := buildBundleArchive(map[string]string{
		"bundle/exasol-nano-db.run":    string(runBytes),
		"bundle/vmlinux.container":     string(kernelBytes),
		"bundle/ubuntu-initrd.cpio.gz": string(initrdBytes),
	})
	if err != nil {
		t.Fatalf("expected embedded bundle fixture, got %v", err)
	}

	metadataJSON := []byte(`{
	  "version": "1.2.3",
	  "architecture": "arm64",
	  "run": {"filename": "exasol-nano-db.run", "sha256": "` + sha256Hex(runBytes) + `"},
	  "boot": {
	    "kernel": {"filename": "vmlinux.container", "sha256": "` + sha256Hex(kernelBytes) + `"},
	    "initrd": {"filename": "ubuntu-initrd.cpio.gz", "sha256": "` + sha256Hex(initrdBytes) + `"}
	  }
	}`)

	payload, err := LoadEmbeddedPayload(metadataJSON, bundleBytes, "arm64")
	if err != nil {
		t.Fatalf("expected embedded payload to load, got %v", err)
	}
	if payload.Metadata.Version != "1.2.3" {
		t.Fatalf("unexpected version: %q", payload.Metadata.Version)
	}

	_, err = LoadEmbeddedPayload(metadataJSON, bundleBytes, "x86_64")
	if err == nil {
		t.Fatal("expected architecture mismatch to fail")
	}
	if !errors.Is(err, ErrEmbeddedPayloadInvalid) {
		t.Fatalf("expected embedded payload invalid error, got %v", err)
	}
}

func TestSeedEmbeddedPayload_ExtractsVerifiesAndReusesCache(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	runBytes := []byte("runfile")
	kernelBytes := []byte("kernel")
	initrdBytes := []byte("initrd")

	bundleBytes, err := buildBundleArchive(map[string]string{
		"bundle/exasol-nano-db.run":    string(runBytes),
		"bundle/vmlinux.container":     string(kernelBytes),
		"bundle/ubuntu-initrd.cpio.gz": string(initrdBytes),
	})
	if err != nil {
		t.Fatalf("expected embedded bundle fixture, got %v", err)
	}

	payload, err := LoadEmbeddedPayload([]byte(`{
	  "version": "1.2.3",
	  "architecture": "arm64",
	  "run": {"filename": "exasol-nano-db.run", "sha256": "`+sha256Hex(runBytes)+`"},
	  "boot": {
	    "kernel": {"filename": "vmlinux.container", "sha256": "`+sha256Hex(kernelBytes)+`"},
	    "initrd": {"filename": "ubuntu-initrd.cpio.gz", "sha256": "`+sha256Hex(initrdBytes)+`"}
	  }
	}`), bundleBytes, "arm64")
	if err != nil {
		t.Fatalf("expected embedded payload to load, got %v", err)
	}

	first, err := SeedEmbeddedPayload(cacheDir, payload)
	if err != nil {
		t.Fatalf("expected embedded payload seeding to succeed, got %v", err)
	}
	second, err := SeedEmbeddedPayload(cacheDir, payload)
	if err != nil {
		t.Fatalf("expected embedded payload cache reuse to succeed, got %v", err)
	}
	if first.RunPath != second.RunPath {
		t.Fatalf("expected run path reuse, got %q then %q", first.RunPath, second.RunPath)
	}
	if _, statErr := os.Stat(first.RunPath); statErr != nil {
		t.Fatalf("expected run path to exist, got %v", statErr)
	}
	if first.Boot == nil {
		t.Fatal("expected boot asset paths to be present")
	}
}

func TestSeedEmbeddedPayload_RejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	runBytes := []byte("runfile")
	kernelBytes := []byte("kernel")
	initrdBytes := []byte("initrd")

	bundleBytes, err := buildBundleArchive(map[string]string{
		"bundle/exasol-nano-db.run":    string(runBytes),
		"bundle/vmlinux.container":     string(kernelBytes),
		"bundle/ubuntu-initrd.cpio.gz": string(initrdBytes),
	})
	if err != nil {
		t.Fatalf("expected embedded bundle fixture, got %v", err)
	}

	payload, err := LoadEmbeddedPayload([]byte(`{
	  "version": "1.2.3",
	  "architecture": "arm64",
	  "run": {"filename": "exasol-nano-db.run", "sha256": "`+sha256Hex([]byte("wrong"))+`"},
	  "boot": {
	    "kernel": {"filename": "vmlinux.container", "sha256": "`+sha256Hex(kernelBytes)+`"},
	    "initrd": {"filename": "ubuntu-initrd.cpio.gz", "sha256": "`+sha256Hex(initrdBytes)+`"}
	  }
	}`), bundleBytes, "arm64")
	if err != nil {
		t.Fatalf("expected embedded payload to load, got %v", err)
	}

	_, err = SeedEmbeddedPayload(cacheDir, payload)
	if err == nil {
		t.Fatal("expected checksum mismatch to fail")
	}
	if !errors.Is(err, ErrPayloadVerificationFailed) {
		t.Fatalf("expected verification error, got %v", err)
	}
}

func buildBundleArchive(files map[string]string) ([]byte, error) {
	path := filepath.Join(os.TempDir(), "embedded-bundle-test.tar.gz")
	file, err := os.CreateTemp("", "embedded-bundle-*.tar.gz")
	if err != nil {
		return nil, err
	}
	path = file.Name()
	_ = file.Close()
	defer os.Remove(path)

	if err := writeTarGzArchive(path, files); err != nil {
		return nil, err
	}

	return os.ReadFile(path)
}
