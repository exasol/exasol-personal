// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type TarGzExtractor struct{}

func (*TarGzExtractor) CanExtract(filename string) bool {
	return strings.HasSuffix(filename, ".tar.gz") || strings.HasSuffix(filename, ".tgz")
}

func (*TarGzExtractor) Extract(srcPath, dstPath string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()

	if err := os.MkdirAll(dstPath, dirPerm); err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)
	extracted := false
	for {
		hdr, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		ok, err := extractTarEntry(tarReader, hdr, dstPath)
		if err != nil {
			return err
		}
		if ok {
			extracted = true
		}
	}

	if !extracted {
		return fmt.Errorf("no extractable entries found in archive %s", srcPath)
	}

	return nil
}

// extractTarEntry extracts a single tar entry under dstPath and reports
// whether it produced an extractable file or directory.
func extractTarEntry(tarReader *tar.Reader, hdr *tar.Header, dstPath string) (bool, error) {
	targetPath, err := sanitizeTarEntryPath(dstPath, hdr.Name)
	if err != nil {
		return false, err
	}

	mode := os.FileMode(hdr.Mode).Perm()

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(targetPath, mode); err != nil {
			return false, err
		}
		if err := os.Chmod(targetPath, mode); err != nil {
			return false, err
		}

		return true, nil
	case tar.TypeReg:
		if err := writeTarRegularFile(tarReader, targetPath, mode); err != nil {
			return false, err
		}

		return true, nil
	default:
		return false, nil
	}
}

func sanitizeTarEntryPath(dstPath, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || cleanName == ".." ||
		strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(cleanName) {
		return "", fmt.Errorf(
			"refusing to extract archive entry %q outside %s",
			name,
			dstPath,
		)
	}

	return filepath.Join(dstPath, cleanName), nil
}

func writeTarRegularFile(tarReader *tar.Reader, targetPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
		return err
	}

	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	// #nosec G110 -- archive contents are trusted runtime artifacts.
	if _, err := io.Copy(out, tarReader); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Chmod(targetPath, mode)
}
