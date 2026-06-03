// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func downloadArtifact(
	ctx context.Context,
	resourceID, url, artifactPath, expectedSha256 string,
) error {
	if err := os.MkdirAll(filepath.Dir(artifactPath), dirPerm); err != nil {
		return err
	}

	tmpDownload, err := os.CreateTemp(filepath.Dir(artifactPath), "download-*")
	if err != nil {
		return err
	}
	tmpDownloadPath := tmpDownload.Name()
	if err := tmpDownload.Close(); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}

	if err := downloadFile(ctx, url, tmpDownloadPath); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}

	actual, err := sha256OfFile(tmpDownloadPath)
	if err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}
	expected := strings.ToLower(strings.TrimSpace(expectedSha256))
	if actual != expected {
		_ = os.Remove(tmpDownloadPath)
		return checksumMismatchError(resourceID, expected, actual)
	}

	_ = os.Remove(artifactPath)
	if err := os.Rename(tmpDownloadPath, artifactPath); err != nil {
		_ = os.Remove(tmpDownloadPath)
		return err
	}

	return nil
}

func extractArtifact(archivePath, resourcePath string) (string, error) {
	extractedRoot, err := archiveExtractionRoot(archivePath)
	if err != nil {
		return "", err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = gzipReader.Close()
	}()

	if err := os.MkdirAll(extractedRoot, dirPerm); err != nil {
		return "", err
	}

	tarReader := tar.NewReader(gzipReader)
	extracted := false
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		cleanName := filepath.Clean(filepath.FromSlash(hdr.Name))
		if cleanName == "." || cleanName == ".." ||
			strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(cleanName) {
			return "", fmt.Errorf(
				"refusing to extract archive entry %q outside %s",
				hdr.Name,
				extractedRoot,
			)
		}

		targetPath := filepath.Join(extractedRoot, cleanName)
		mode := os.FileMode(hdr.Mode).Perm()

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, mode); err != nil {
				return "", err
			}
			if err := os.Chmod(targetPath, mode); err != nil {
				return "", err
			}
			extracted = true
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), dirPerm); err != nil {
				return "", err
			}

			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return "", err
			}
			// #nosec G110 -- archive contents are trusted runtime artifacts.
			if _, err := io.Copy(out, tarReader); err != nil {
				_ = out.Close()
				return "", err
			}
			if err := out.Close(); err != nil {
				return "", err
			}
			if err := os.Chmod(targetPath, mode); err != nil {
				return "", err
			}
			extracted = true
		default:
			continue
		}
	}

	if !extracted {
		return "", fmt.Errorf("no extractable entries found in archive %s", archivePath)
	}

	resolvedPath, err := extractedResourcePath(extractedRoot, resourcePath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(resolvedPath); err != nil {
		return "", err
	}

	return resolvedPath, nil
}

func downloadFile(ctx context.Context, url, destPath string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("unsupported resource URL scheme in %q", url)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, resp.Body)

	return err
}

func sha256OfFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func checksumMismatchError(resourceID, expected, actual string) error {
	return fmt.Errorf(
		"resource %q checksum mismatch: expected %s, got %s",
		resourceID,
		expected,
		actual,
	)
}

func archiveExtractionRoot(archivePath string) (string, error) {
	outputDir := filepath.Dir(archivePath)
	archiveName := filepath.Base(archivePath)
	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"):
		archiveName = strings.TrimSuffix(archiveName, ".tar.gz")
	case strings.HasSuffix(archiveName, ".tgz"):
		archiveName = strings.TrimSuffix(archiveName, ".tgz")
	default:
		return "", fmt.Errorf("extraction not implemented for %s", archivePath)
	}

	return filepath.Join(outputDir, archiveName), nil
}
