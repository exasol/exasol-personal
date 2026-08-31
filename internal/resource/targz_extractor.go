// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

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

const tarPermissionMask = 0o777

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

func extractTarEntry(
	tarReader *tar.Reader,
	hdr *tar.Header,
	dstPath string,
) (bool, error) {
	root, err := os.OpenRoot(dstPath)
	if err != nil {
		return false, err
	}
	defer root.Close()

	// Clean strips the trailing separator tar uses to mark a directory entry;
	// os.Root rejects such a name outright.
	targetPath := filepath.Clean(filepath.FromSlash(hdr.Name))
	if targetPath == "." || targetPath == string(filepath.Separator) {
		return false, fmt.Errorf("invalid archive entry %q", hdr.Name)
	}

	mode := os.FileMode(hdr.Mode & tarPermissionMask).Perm()

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := root.MkdirAll(targetPath, mode); err != nil {
			return false, err
		}
		if err := root.Chmod(targetPath, mode); err != nil {
			return false, err
		}

		return true, nil

	case tar.TypeReg:
		if err := writeTarRegularFile(tarReader, root, targetPath, mode); err != nil {
			return false, err
		}

		return true, nil

	case tar.TypeSymlink:
		if err := writeTarSymlink(root, hdr, targetPath); err != nil {
			return false, err
		}

		return true, nil

	default:
		return false, nil
	}
}

func writeTarSymlink(root *os.Root, hdr *tar.Header, targetPath string) error {
	linkTarget := filepath.FromSlash(hdr.Linkname)
	cleanTarget := filepath.Clean(linkTarget)

	// Do not create links that point outside the extraction root.
	if filepath.IsAbs(cleanTarget) ||
		cleanTarget == ".." ||
		strings.HasPrefix(cleanTarget, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing unsafe symlink target %q", hdr.Linkname)
	}

	if err := root.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
		return err
	}
	if err := root.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return root.Symlink(linkTarget, targetPath)
}

func writeTarRegularFile(
	tarReader *tar.Reader,
	root *os.Root,
	targetPath string,
	mode os.FileMode,
) error {
	if err := root.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
		return err
	}

	out, err := root.OpenFile(
		targetPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		mode,
	)
	if err != nil {
		return err
	}

	// #nosec G110 -- archive contents are trusted resources.
	if _, err := io.Copy(out, tarReader); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return root.Chmod(targetPath, mode)
}
