// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package assets

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrPayloadBundleInvalid = errors.New("local runtime payload bundle is invalid")

const (
	bundleDirMode  = 0o700
	bundleFileMode = 0o600
)

type Bundle struct {
	RootDir    string
	KernelPath string
	InitrdPath string
}

func PrepareBundle(sourcePath string, destinationRoot string) (*Bundle, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat payload bundle source: %w", err)
	}

	if sourceInfo.IsDir() {
		return discoverBundle(sourcePath)
	}

	if bundle, err := discoverBundle(destinationRoot); err == nil {
		return bundle, nil
	}

	if err := os.RemoveAll(destinationRoot); err != nil {
		return nil, fmt.Errorf("failed to reset payload extraction dir: %w", err)
	}
	if err := os.MkdirAll(destinationRoot, bundleDirMode); err != nil {
		return nil, fmt.Errorf("failed to create payload extraction dir: %w", err)
	}

	if err := extractTarGz(sourcePath, destinationRoot); err != nil {
		return nil, err
	}

	return discoverBundle(destinationRoot)
}

func discoverBundle(rootDir string) (*Bundle, error) {
	kernelPath, err := findBundleFile(rootDir, "vmlinux.container")
	if err != nil {
		return nil, err
	}

	initrdPath, err := findBundleFile(rootDir, "ubuntu-initrd.cpio.gz")
	if err != nil {
		return nil, err
	}

	return &Bundle{
		RootDir:    rootDir,
		KernelPath: kernelPath,
		InitrdPath: initrdPath,
	}, nil
}

func findBundleFile(rootDir string, targetName string) (string, error) {
	var resolvedPath string

	err := filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Base(path) == targetName {
			resolvedPath = path
			return filepath.SkipAll
		}

		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return "", fmt.Errorf("failed to inspect payload bundle: %w", err)
	}
	if resolvedPath == "" {
		return "", fmt.Errorf("%w: missing %s", ErrPayloadBundleInvalid, targetName)
	}

	return resolvedPath, nil
}

func extractTarGz(sourcePath string, destinationRoot string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open payload bundle archive: %w", err)
	}
	defer sourceFile.Close()

	gzipReader, err := gzip.NewReader(sourceFile)
	if err != nil {
		return fmt.Errorf(
			"%w: payload bundle is not a gzip archive: %w",
			ErrPayloadBundleInvalid,
			err,
		)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf(
				"%w: failed to read payload bundle archive: %w",
				ErrPayloadBundleInvalid,
				err,
			)
		}

		targetPath, err := safeBundlePath(destinationRoot, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, bundleDirMode); err != nil {
				return fmt.Errorf("failed to create payload bundle dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), bundleDirMode); err != nil {
				return fmt.Errorf("failed to create payload bundle parent dir: %w", err)
			}

			file, err := os.OpenFile(
				targetPath,
				os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
				fileMode(header.FileInfo().Mode()),
			)
			if err != nil {
				return fmt.Errorf("failed to create payload bundle file: %w", err)
			}

			if _, err := io.CopyN(file, tarReader, header.Size); err != nil {
				_ = file.Close()
				return fmt.Errorf("failed to extract payload bundle file: %w", err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("failed to finalize payload bundle file: %w", err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), bundleDirMode); err != nil {
				return fmt.Errorf("failed to create payload bundle symlink dir: %w", err)
			}
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return fmt.Errorf("failed to extract payload bundle symlink: %w", err)
			}
		default:
			return fmt.Errorf(
				"%w: unsupported tar entry type %d for %s",
				ErrPayloadBundleInvalid,
				header.Typeflag,
				header.Name,
			)
		}
	}
}

func safeBundlePath(destinationRoot string, name string) (string, error) {
	cleanName := filepath.Clean(strings.TrimPrefix(name, "/"))
	targetPath := filepath.Join(destinationRoot, cleanName)

	relativePath, err := filepath.Rel(destinationRoot, targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve payload bundle path: %w", err)
	}
	if strings.HasPrefix(relativePath, "..") {
		return "", fmt.Errorf("%w: invalid payload bundle path %q", ErrPayloadBundleInvalid, name)
	}

	return targetPath, nil
}

func fileMode(mode fs.FileMode) fs.FileMode {
	if mode == 0 {
		return bundleFileMode
	}

	return mode.Perm()
}
