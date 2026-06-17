// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ZipExtractor struct{}

func (*ZipExtractor) CanExtract(filename string) bool {
	return strings.HasSuffix(filename, ".zip")
}

func (*ZipExtractor) Extract(srcPath, dstPath string) error {
	zipReader, err := zip.OpenReader(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = zipReader.Close() }()

	if err := os.MkdirAll(dstPath, dirPerm); err != nil {
		return err
	}

	extracted := false
	for _, zipEntry := range zipReader.File {
		cleanName := filepath.Clean(filepath.FromSlash(zipEntry.Name))
		if cleanName == "." || cleanName == ".." ||
			strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(cleanName) {
			return fmt.Errorf(
				"refusing to extract archive entry %q outside %s",
				zipEntry.Name,
				dstPath,
			)
		}

		targetPath := filepath.Join(dstPath, cleanName)
		mode := zipEntry.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}

		if zipEntry.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, mode); err != nil {
				return err
			}
			extracted = true

			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
			return err
		}

		entryReader, err := zipEntry.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			_ = entryReader.Close()
			return err
		}
		// #nosec G110 -- archive contents are trusted runtime artifacts.
		if _, err := io.Copy(out, entryReader); err != nil {
			_ = entryReader.Close()
			_ = out.Close()

			return err
		}
		_ = entryReader.Close()
		if err := out.Close(); err != nil {
			return err
		}
		extracted = true
	}

	if !extracted {
		return fmt.Errorf("no extractable entries found in archive %s", srcPath)
	}

	return nil
}
