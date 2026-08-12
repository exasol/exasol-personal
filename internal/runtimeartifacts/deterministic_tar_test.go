// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepackTarDeterministically_NormalizesHostMetadata(t *testing.T) {
	t.Parallel()

	// Given
	inputModes := []int64{0o644, 0o755}
	outputs := make([][]byte, 0, len(inputModes))
	for index, mode := range inputModes {
		srcPath := filepath.Join(t.TempDir(), "source.tar")
		dstPath := filepath.Join(t.TempDir(), "repacked.tar")
		writeTarFixture(t, srcPath, mode, time.Unix(int64(index+1), 0).UTC())

		// When
		if err := repackTarDeterministically(srcPath, dstPath); err != nil {
			t.Fatalf("repack archive with mode %o: %v", mode, err)
		}

		// Then
		output, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("read repacked archive: %v", err)
		}
		outputs = append(outputs, output)
		assertDeterministicTarMetadata(t, output)
	}

	if !bytes.Equal(outputs[0], outputs[1]) {
		t.Fatal("expected archives with different host metadata to be repacked identically")
	}
}

func TestRepackTarDeterministically_RemovesIncompleteOutput(t *testing.T) {
	t.Parallel()

	// Given
	srcPath := filepath.Join(t.TempDir(), "invalid.tar")
	dstPath := filepath.Join(t.TempDir(), "repacked.tar")
	if err := os.WriteFile(srcPath, []byte("not a tar archive"), filePerm); err != nil {
		t.Fatalf("write invalid archive: %v", err)
	}

	// When
	err := repackTarDeterministically(srcPath, dstPath)

	// Then
	if err == nil {
		t.Fatal("expected invalid tar archive to fail")
	}
	if _, statErr := os.Stat(dstPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected incomplete output to be removed, got %v", statErr)
	}
}

func writeTarFixture(t *testing.T, path string, mode int64, timestamp time.Time) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar fixture: %v", err)
	}
	writer := tar.NewWriter(file)
	headers := []*tar.Header{
		{
			Name:       "blobs/",
			Typeflag:   tar.TypeDir,
			Mode:       mode,
			Uid:        1000,
			Gid:        1000,
			Uname:      "host-user",
			Gname:      "host-group",
			ModTime:    timestamp,
			AccessTime: timestamp,
			ChangeTime: timestamp,
			Format:     tar.FormatPAX,
		},
		{
			Name:       "blobs/image",
			Typeflag:   tar.TypeReg,
			Mode:       mode,
			Uid:        1000,
			Gid:        1000,
			Uname:      "host-user",
			Gname:      "host-group",
			Size:       int64(len("image contents")),
			ModTime:    timestamp,
			AccessTime: timestamp,
			ChangeTime: timestamp,
			Format:     tar.FormatPAX,
		},
	}
	for _, header := range headers {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write tar fixture header: %v", err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := io.WriteString(writer, "image contents"); err != nil {
				t.Fatalf("write tar fixture contents: %v", err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar fixture writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close tar fixture: %v", err)
	}
}

func assertDeterministicTarMetadata(t *testing.T, data []byte) {
	t.Helper()

	reader := tar.NewReader(bytes.NewReader(data))
	entryCount := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read repacked archive: %v", err)
		}
		entryCount++
		if header.Mode != deterministicTarMode {
			t.Errorf("entry %q mode = %o, want %o", header.Name, header.Mode, deterministicTarMode)
		}
		if header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" {
			t.Errorf(
				"entry %q ownership = %d:%d %q:%q, want zero values",
				header.Name,
				header.Uid,
				header.Gid,
				header.Uname,
				header.Gname,
			)
		}
		if !header.ModTime.Equal(deterministicTarTimestamp) {
			t.Errorf(
				"entry %q modification time = %s, want %s",
				header.Name,
				header.ModTime,
				deterministicTarTimestamp,
			)
		}
		if !header.AccessTime.IsZero() || !header.ChangeTime.IsZero() {
			t.Errorf(
				"entry %q access/change times = %s/%s, want zero values",
				header.Name,
				header.AccessTime,
				header.ChangeTime,
			)
		}
		if header.Format != tar.FormatUSTAR {
			t.Errorf("entry %q format = %v, want USTAR", header.Name, header.Format)
		}
	}
	if entryCount != 2 {
		t.Fatalf("repacked archive entry count = %d, want 2", entryCount)
	}
}
