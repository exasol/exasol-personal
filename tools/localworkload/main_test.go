// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/assets/localworkloadbin"
)

func TestStageWritesDigestPinnedPlatformArtifact(t *testing.T) {
	t.Parallel()

	// Given
	root := t.TempDir()
	archive := filepath.Join(t.TempDir(), "nano.tar.gz")
	digest, archiveData := testOCIArchive(t, "linux", "amd64")
	if err := os.WriteFile(archive, archiveData, 0o600); err != nil {
		t.Fatal(err)
	}
	reference := "docker.io/exasol/nano@" + digest

	// When
	if err := run([]string{
		"stage",
		"-goos", "windows",
		"-goarch", "amd64",
		"-root", root,
		"-archive", archive,
		"-reference", reference,
	}); err != nil {
		t.Fatal(err)
	}

	// Then
	metadataData, err := os.ReadFile(filepath.Join(root, "windows", "amd64", "image.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata localworkloadbin.Metadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.ImageReference != reference || metadata.ImageDigest != digest ||
		metadata.ArchiveSHA256 != fmt.Sprintf(
			"sha256:%x",
			sha256.Sum256(archiveData),
		) ||
		metadata.Platform != "windows/amd64" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
}

func TestStageRejectsArchiveWithWrongPlatform(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	archive := filepath.Join(t.TempDir(), "nano.tar.gz")
	digest, data := testOCIArchive(t, "linux", "arm64")
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"stage",
		"-goos", "windows",
		"-goarch", "amd64",
		"-root", root,
		"-archive", archive,
		"-reference", "example.test/nano@" + digest,
	})
	if err == nil {
		t.Fatal("expected wrong-platform OCI image to be rejected")
	}
}

func TestStageUsesImageConfigWhenOCIDescriptorHasNoPlatform(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	archive := filepath.Join(t.TempDir(), "nano.tar.gz")
	digest, data := testOCIArchiveConfig(t, "linux", "amd64", "/controller", false)
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"stage",
		"-goos", "windows",
		"-goarch", "amd64",
		"-root", root,
		"-archive", archive,
		"-reference", "example.test/nano@" + digest,
	})
	if err != nil {
		t.Fatalf("expected single-platform OCI archive to be accepted, got %v", err)
	}
}

func TestStageRejectsUnexpectedNanoEntrypoint(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	archive := filepath.Join(t.TempDir(), "nano.tar.gz")
	digest, data := testOCIArchiveConfig(t, "linux", "amd64", "/bin/sh", true)
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}

	err := run([]string{
		"stage",
		"-goos", "windows",
		"-goarch", "amd64",
		"-root", root,
		"-archive", archive,
		"-reference", "example.test/nano@" + digest,
	})
	if err == nil {
		t.Fatal("expected unexpected Nano entrypoint to be rejected")
	}
}

func testOCIArchive(t *testing.T, goos, goarch string) (string, []byte) {
	t.Helper()
	return testOCIArchiveConfig(t, goos, goarch, "/controller", true)
}

func testOCIArchiveConfig(
	t *testing.T,
	goos, goarch, entrypoint string,
	includeDescriptorPlatform bool,
) (string, []byte) {
	t.Helper()
	config := []byte(fmt.Sprintf(
		`{"os":%q,"architecture":%q,"config":{"Entrypoint":[%q]}}`,
		goos,
		goarch,
		entrypoint,
	))
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(config))
	layer := []byte("layer")
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layer))
	manifest := []byte(fmt.Sprintf(
		`{"schemaVersion":2,"config":{"digest":%q},"layers":[{"digest":%q}]}`,
		configDigest,
		layerDigest,
	))
	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	descriptor := fmt.Sprintf(`{"digest":%q}`, manifestDigest)
	if includeDescriptorPlatform {
		descriptor = fmt.Sprintf(
			`{"digest":%q,"platform":{"os":%q,"architecture":%q}}`,
			manifestDigest,
			goos,
			goarch,
		)
	}
	index := []byte(fmt.Sprintf(
		`{"schemaVersion":2,"manifests":[%s]}`,
		descriptor,
	))
	files := map[string][]byte{
		"oci-layout":                         []byte(`{"imageLayoutVersion":"1.0.0"}`),
		"index.json":                         index,
		"blobs/sha256/" + configDigest[7:]:   config,
		"blobs/sha256/" + layerDigest[7:]:    layer,
		"blobs/sha256/" + manifestDigest[7:]: manifest,
	}
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for name, data := range files {
		if err := writer.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(data)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}

	return manifestDigest, buffer.Bytes()
}

func TestReleaseStageRequiresRealImageInput(t *testing.T) {
	t.Parallel()

	// When
	err := run([]string{
		"stage",
		"-goos", "darwin",
		"-goarch", "arm64",
		"-root", t.TempDir(),
		"-release-build",
	})

	// Then
	if err == nil {
		t.Fatal("expected missing release image to be rejected")
	}
}
