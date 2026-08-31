// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package resource

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

// FileURLScheme is the URL scheme identifying a local filesystem artifact.
const FileURLScheme = "file://"

type FileSource struct{}

func (FileSource) CanFetch(url string) bool {
	if strings.HasPrefix(url, FileURLScheme) {
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
	return resolveLocalPath(url)
}

func resolveLocalPath(url string) (string, error) {
	rawPath := strings.TrimPrefix(url, FileURLScheme)
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
