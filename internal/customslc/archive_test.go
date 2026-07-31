// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package customslc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestValidateArchiveAcceptsValidSLC(t *testing.T) {
	t.Parallel()

	// Given
	archive := gzipBytes(t, buildTar(t, []archiveEntry{
		{name: "exaudf/exaudfclient", body: "#!/bin/sh\n", mode: 0o755},
		{name: "python/runtime", body: "x", mode: 0o644},
	}))

	// When
	err := ValidateArchive(bytes.NewReader(archive))
	// Then
	if err != nil {
		t.Fatalf("expected a valid SLC to pass, got %v", err)
	}
}

func TestValidateArchiveAcceptsUncompressedTar(t *testing.T) {
	t.Parallel()

	// Given
	archive := buildTar(t, []archiveEntry{
		{name: "exaudf/exaudfclient", body: "#!/bin/sh\n", mode: 0o755},
	})

	// When
	err := ValidateArchive(bytes.NewReader(archive))
	// Then
	if err != nil {
		t.Fatalf("expected an uncompressed tar to pass, got %v", err)
	}
}

func TestValidateArchiveRejectsMissingClient(t *testing.T) {
	t.Parallel()

	// Given
	archive := gzipBytes(t, buildTar(t, []archiveEntry{
		{name: "readme.txt", body: "not an slc", mode: 0o644},
	}))

	// When
	err := ValidateArchive(bytes.NewReader(archive))
	// Then
	if err == nil || !strings.Contains(err.Error(), clientRelPath) {
		t.Fatalf("expected a missing-client error naming %s, got %v", clientRelPath, err)
	}
}

func TestValidateArchiveRejectsNonExecutableClient(t *testing.T) {
	t.Parallel()

	// Given
	archive := gzipBytes(t, buildTar(t, []archiveEntry{
		{name: "exaudf/exaudfclient", body: "x", mode: 0o644},
	}))

	// When
	err := ValidateArchive(bytes.NewReader(archive))
	// Then
	if err == nil {
		t.Fatal("expected a non-executable client to be rejected")
	}
}

func TestValidateArchiveDetectsCorruptGzip(t *testing.T) {
	t.Parallel()

	// Given
	archive := gzipBytes(t, buildTar(t, []archiveEntry{
		{name: "exaudf/exaudfclient", body: strings.Repeat("payload", 100), mode: 0o755},
	}))
	corrupted := append([]byte(nil), archive...)
	corrupted[len(corrupted)/2] ^= 0xff

	// When
	err := ValidateArchive(bytes.NewReader(corrupted))
	// Then
	if err == nil {
		t.Fatal("expected corrupt gzip content to be rejected")
	}
}

type archiveEntry struct {
	name string
	body string
	mode int64
}

func buildTar(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	tarWriter := tar.NewWriter(&buf)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Typeflag: tar.TypeReg,
			Size:     int64(len(entry.body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}
