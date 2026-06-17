// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeartifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileSource struct{}

func (FileSource) CanFetch(url string) bool {
	if strings.HasPrefix(url, "file://") {
		return true
	}
	// Local filesystem path: no URL scheme and not a git@ remote
	return !strings.Contains(url, "://") && !strings.HasPrefix(url, "git@")
}

func (FileSource) Identify(_ context.Context, url string) (string, error) {
	absPath, err := resolveLocalPath(url)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(absPath))

	return hex.EncodeToString(sum[:]), nil
}

func (FileSource) Fetch(_ context.Context, url string, _ string) (string, error) {
	absPath, err := resolveLocalPath(url)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() && !isSupportedLocalArchive(absPath) {
		return "", fmt.Errorf(
			"resource path must be a directory or a supported archive file: %s", absPath,
		)
	}

	return absPath, nil
}

func resolveLocalPath(url string) (string, error) {
	rawPath := strings.TrimPrefix(url, "file://")
	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resource path does not exist: %s", absPath)
		}

		return "", err
	}

	return resolved, nil
}

func isSupportedLocalArchive(path string) bool {
	return strings.HasSuffix(path, ".tar.gz") ||
		strings.HasSuffix(path, ".tgz") ||
		strings.HasSuffix(path, ".zip")
}
