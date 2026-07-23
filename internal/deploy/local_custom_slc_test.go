// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
)

func TestExtractTarOverSFTP(t *testing.T) {
	t.Parallel()

	// Given
	client := newLoopbackSFTPClient(t)
	target := filepath.Join(t.TempDir(), "slc")
	archive := buildTarGz(t, []tarEntry{
		{name: "exaudf/", dir: true},
		{name: "exaudf/exaudfclient", body: "#!/bin/sh\n", mode: 0o755},
		{name: "exaudf/lib/data.txt", body: "hello", mode: 0o644},
		{name: "exaudf/link", link: "exaudfclient"},
	})

	// When
	err := extractTarOverSFTP(client, bytes.NewReader(archive), target)
	// Then
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(target, "exaudf/lib/data.txt"))
	if err != nil || string(body) != "hello" {
		t.Fatalf("regular file not extracted: %q err %v", body, err)
	}
	linkDest, err := os.Readlink(filepath.Join(target, "exaudf/link"))
	if err != nil || linkDest != "exaudfclient" {
		t.Fatalf("symlink not extracted: %q err %v", linkDest, err)
	}
}

// Re-extracting into the same directory must replace the previous contents so install and
// replace stay idempotent.
func TestExtractTarOverSFTPReplacesExisting(t *testing.T) {
	t.Parallel()

	client := newLoopbackSFTPClient(t)
	target := filepath.Join(t.TempDir(), "slc")

	first := buildTarGz(t, []tarEntry{{name: "stale.txt", body: "old", mode: 0o644}})
	if err := extractTarOverSFTP(client, bytes.NewReader(first), target); err != nil {
		t.Fatal(err)
	}
	second := buildTarGz(t, []tarEntry{{name: "fresh.txt", body: "new", mode: 0o644}})
	if err := extractTarOverSFTP(client, bytes.NewReader(second), target); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file survived replace: %v", err)
	}
}

// extractTarOverSFTP re-checks per-entry safety as defense in depth, so a traversal entry is
// rejected even if host-side validation were bypassed.
func TestExtractTarOverSFTPRejectsTraversal(t *testing.T) {
	t.Parallel()

	// Given
	client := newLoopbackSFTPClient(t)
	target := filepath.Join(t.TempDir(), "slc")
	archive := buildTarGz(t, []tarEntry{{name: "../escape.txt", body: "x", mode: 0o644}})

	// When
	err := extractTarOverSFTP(client, bytes.NewReader(archive), target)
	// Then
	if err == nil {
		t.Fatal("expected extraction to reject a path-traversal entry")
	}
}

func TestExtractTarOverSFTPCreatesParentForSymlink(t *testing.T) {
	t.Parallel()

	// Given
	client := newLoopbackSFTPClient(t)
	target := filepath.Join(t.TempDir(), "slc")
	archive := buildTarGz(t, []tarEntry{{name: "nested/link", link: "target"}})

	// When
	err := extractTarOverSFTP(client, bytes.NewReader(archive), target)
	// Then
	if err != nil {
		t.Fatalf("a nested symlink with no parent dir entry should extract: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "nested", "link")); err != nil {
		t.Fatalf("symlink not created under a missing parent dir: %v", err)
	}
}

// A symlink to an outside directory followed by a write under it must be rejected before any
// bytes land outside the target, closing the write-through-symlink extraction escape.
func TestExtractTarOverSFTPRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	// Given
	client := newLoopbackSFTPClient(t)
	target := filepath.Join(t.TempDir(), "slc")
	outside := t.TempDir()
	archive := buildTarGz(t, []tarEntry{
		{name: "escape", link: outside},
		{name: "escape/pwned", body: "owned", mode: 0o644},
	})

	// When
	err := extractTarOverSFTP(client, bytes.NewReader(archive), target)
	// Then
	if err == nil {
		t.Fatal("expected extraction to reject a write through a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("write escaped the target directory: %v", err)
	}
}

func TestRemoveAllIfExists(t *testing.T) {
	t.Parallel()

	client := newLoopbackSFTPClient(t)

	if err := removeAllIfExists(client, filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Fatalf("a missing path must be tolerated, got %v", err)
	}

	_ = client.Close()
	if err := removeAllIfExists(client, filepath.Join(t.TempDir(), "any")); err == nil {
		t.Fatal("a non-not-exist Stat failure must be propagated, not swallowed")
	}
}

type tarEntry struct {
	name string
	body string
	link string
	mode int64
	dir  bool
}

func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: entry.mode}
		switch {
		case entry.dir:
			header.Typeflag = tar.TypeDir
		case entry.link != "":
			header.Typeflag = tar.TypeSymlink
			header.Linkname = entry.link
		default:
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(entry.body))
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// newLoopbackSFTPClient wires an SFTP client to an in-process SFTP server serving the real
// filesystem over an in-memory pipe, so extraction is exercised against a genuine SFTP
// backend without a network or a remote host.
func newLoopbackSFTPClient(t *testing.T) *sftp.Client {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	server, err := sftp.NewServer(serverConn)
	if err != nil {
		t.Fatalf("failed to start sftp server: %v", err)
	}
	go func() { _ = server.Serve() }()

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatalf("failed to start sftp client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	return client
}
