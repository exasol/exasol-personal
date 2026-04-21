// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/assets/tofubin"
)

// BinaryName is the platform-specific tofu binary name.
const (
	BinaryName        = tofubin.TofuBinaryName
	maxTofuBinarySize = 512 * 1024 * 1024 // 512 MiB
)

// EnsureEmbeddedTofuArchiveIsNotPlaceholder validates that the embedded tofu
// payload is available and not a placeholder.
func EnsureEmbeddedTofuArchiveIsNotPlaceholder() error {
	if len(tofubin.TofuArchive) == 0 {
		return errors.New("embedded tofu archive is empty; run `task generate` to download it")
	}
	if len(tofubin.TofuArchive) < 256 &&
		bytes.Contains(bytes.ToLower(tofubin.TofuArchive), []byte("placeholder")) {
		return errors.New(
			"embedded tofu archive appears to be a placeholder; run `task generate` " +
				"(or tofu download task) to fetch OpenTofu",
		)
	}

	return nil
}

// WriteBinary extracts the embedded tofu archive and writes the executable to
// the given path.
func WriteBinary(path string) error {
	const perm = 0o744

	if err := EnsureEmbeddedTofuArchiveIsNotPlaceholder(); err != nil {
		return err
	}

	compressedReader := bytes.NewReader(tofubin.TofuArchive)
	gzipReader, err := gzip.NewReader(compressedReader)
	if err != nil {
		return fmt.Errorf("read embedded tofu archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read embedded tofu archive entries: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != BinaryName {
			continue
		}
		if header.Size <= 0 || header.Size > maxTofuBinarySize {
			return fmt.Errorf(
				"invalid embedded tofu binary size %d bytes",
				header.Size,
			)
		}

		outFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm) // nolint: gosec
		if err != nil {
			return fmt.Errorf("create tofu binary %q: %w", path, err)
		}
		if _, err := io.CopyN(outFile, tarReader, header.Size); err != nil {
			closeErr := outFile.Close()
			if closeErr != nil {
				return fmt.Errorf("close tofu binary %q after write failure: %w", path, closeErr)
			}

			return fmt.Errorf("write tofu binary %q: %w", path, err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("close tofu binary %q: %w", path, err)
		}

		return nil
	}

	return fmt.Errorf(
		"embedded tofu archive does not contain %q (archive entries did not match)",
		BinaryName,
	)
}
