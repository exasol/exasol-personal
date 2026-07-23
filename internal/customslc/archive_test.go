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

func TestCheckEntryRejectsEscapes(t *testing.T) {
	t.Parallel()

	// Given
	for _, testCase := range []struct {
		name    string
		header  *tar.Header
		wantErr bool
	}{
		{"safe file", &tar.Header{Name: "exaudf/exaudfclient", Typeflag: tar.TypeReg}, false},
		{"parent traversal", &tar.Header{Name: "../escape", Typeflag: tar.TypeReg}, true},
		{"absolute", &tar.Header{Name: "/etc/passwd", Typeflag: tar.TypeReg}, true},
		// Symlink targets are not restricted: real containers ship absolute symlinks for
		// their own runtime namespace, so these are accepted and created as-is.
		{
			"absolute symlink allowed",
			&tar.Header{
				Name:     "etc/resolv.conf",
				Linkname: "/conf/resolv.conf",
				Typeflag: tar.TypeSymlink,
			},
			false,
		},
		{
			"escaping symlink target allowed",
			&tar.Header{Name: "link", Linkname: "../../etc", Typeflag: tar.TypeSymlink},
			false,
		},
		{
			"symlink with escaping name rejected",
			&tar.Header{Name: "../link", Linkname: "target", Typeflag: tar.TypeSymlink},
			true,
		},
		{
			"escaping hardlink",
			&tar.Header{Name: "link", Linkname: "../etc", Typeflag: tar.TypeLink},
			true,
		},
	} {
		// When
		err := CheckEntry(testCase.header)
		// Then
		if testCase.wantErr && err == nil {
			t.Fatalf("%s: expected an error", testCase.name)
		}
		if !testCase.wantErr && err != nil {
			t.Fatalf("%s: unexpected error %v", testCase.name, err)
		}
	}
}

func TestEntryCheckerRejectsSymlinkEscapes(t *testing.T) {
	t.Parallel()

	// Given
	for _, testCase := range []struct {
		name    string
		entries []*tar.Header
		wantErr bool
	}{
		{
			name: "leaf absolute symlink accepted",
			entries: []*tar.Header{
				{Name: "etc/resolv.conf", Linkname: "/conf/resolv.conf", Typeflag: tar.TypeSymlink},
				{Name: "exaudf/exaudfclient", Typeflag: tar.TypeReg},
			},
		},
		{
			name: "write under a symlinked dir rejected",
			entries: []*tar.Header{
				{Name: "escape", Linkname: "/tmp", Typeflag: tar.TypeSymlink},
				{Name: "escape/file", Typeflag: tar.TypeReg},
			},
			wantErr: true,
		},
		{
			name: "write at a symlink path rejected",
			entries: []*tar.Header{
				{Name: "escape", Linkname: "/tmp/file", Typeflag: tar.TypeSymlink},
				{Name: "escape", Typeflag: tar.TypeReg},
			},
			wantErr: true,
		},
		{
			name: "hardlink resolving through a symlink rejected",
			entries: []*tar.Header{
				{Name: "dir", Linkname: "/tmp", Typeflag: tar.TypeSymlink},
				{Name: "link", Linkname: "dir/secret", Typeflag: tar.TypeLink},
			},
			wantErr: true,
		},
		{
			name: "nested files without symlinks accepted",
			entries: []*tar.Header{
				{Name: "python", Typeflag: tar.TypeDir},
				{Name: "python/runtime", Typeflag: tar.TypeReg},
			},
		},
	} {
		// When
		checker := NewEntryChecker()
		var err error
		for _, header := range testCase.entries {
			if err = checker.Check(header); err != nil {
				break
			}
		}

		// Then
		if testCase.wantErr && err == nil {
			t.Fatalf("%s: expected an error", testCase.name)
		}
		if !testCase.wantErr && err != nil {
			t.Fatalf("%s: unexpected error %v", testCase.name, err)
		}
	}
}

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

func TestValidateArchiveRejectsTraversal(t *testing.T) {
	t.Parallel()

	// Given
	archive := gzipBytes(t, buildTar(t, []archiveEntry{
		{name: "../escape", body: "x", mode: 0o644},
		{name: "exaudf/exaudfclient", body: "x", mode: 0o755},
	}))

	// When
	err := ValidateArchive(bytes.NewReader(archive))
	// Then
	if err == nil {
		t.Fatal("expected a traversal entry to be rejected")
	}
}

// A gzip stream whose payload bytes are corrupted must fail the CRC check that OpenTar's
// finish step forces by draining to EOF.
func TestValidateArchiveDetectsCorruptGzip(t *testing.T) {
	t.Parallel()

	// Given
	archive := gzipBytes(t, buildTar(t, []archiveEntry{
		{name: "exaudf/exaudfclient", body: strings.Repeat("payload", 100), mode: 0o755},
	}))
	// Flip a byte inside the compressed body (past the 10-byte gzip header).
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
