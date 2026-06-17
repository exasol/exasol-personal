// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"compress/gzip"
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
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(filepath.FromSlash(hdr.Name))
		if cleanName == "." || cleanName == ".." ||
			strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(cleanName) {
			return fmt.Errorf(
				"refusing to extract archive entry %q outside %s",
				hdr.Name,
				dstPath,
			)
		}

		targetPath := filepath.Join(dstPath, cleanName)
		mode := os.FileMode(hdr.Mode).Perm()

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, mode); err != nil {
				return err
			}
			if err := os.Chmod(targetPath, mode); err != nil {
				return err
			}
			extracted = true
		case tar.TypeReg:
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
			if err := os.Chmod(targetPath, mode); err != nil {
				return err
			}
			extracted = true
		default:
			continue
		}
	}

	if !extracted {
		return fmt.Errorf("no extractable entries found in archive %s", srcPath)
	}

	return nil
}
