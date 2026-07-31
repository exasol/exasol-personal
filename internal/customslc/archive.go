// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package customslc

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
)

var gzipMagic = []byte{0x1f, 0x8b}

func openTar(reader io.Reader) (*tar.Reader, func() error, error) {
	buffered := bufio.NewReader(reader)
	magic, err := buffered.Peek(len(gzipMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}
	if errors.Is(err, io.EOF) || !bytes.Equal(magic, gzipMagic) {
		return tar.NewReader(buffered), func() error { return nil }, nil
	}

	gzipReader, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, nil, err
	}
	finish := func() error {
		// Draining to EOF is what makes gzip verify its CRC; the size is unbounded by design
		// (a single-tenant, operator-supplied container of unpredictable size).
		//nolint:gosec // G110: unbounded decompression is intentional (see above).
		if _, err := io.Copy(io.Discard, gzipReader); err != nil {
			return err
		}

		return gzipReader.Close()
	}

	return tar.NewReader(gzipReader), finish, nil
}

func ValidateArchive(reader io.Reader) error {
	tarReader, finish, err := openTar(reader)
	if err != nil {
		return err
	}

	clientFound := false
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if isClientExecutable(header) {
			clientFound = true
		}
	}

	if err := finish(); err != nil {
		return fmt.Errorf("container archive is corrupt: %w", err)
	}
	if !clientFound {
		return fmt.Errorf(
			"%s was not found as an executable in the container; Exasol Personal supports "+
				"standard SLCs built with exaslct",
			clientRelPath,
		)
	}

	return nil
}

func isClientExecutable(header *tar.Header) bool {
	if header.Typeflag != tar.TypeReg {
		return false
	}
	if path.Clean(header.Name) != clientRelPath {
		return false
	}

	return header.FileInfo().Mode().Perm()&0o111 != 0
}
