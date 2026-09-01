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

const FileURLScheme = "file://"

type FileSource struct{}

func (FileSource) Handles(loc Locator) bool {
	if strings.HasPrefix(loc.URL, FileURLScheme) {
		return true
	}

	return !strings.Contains(loc.URL, "://") && !strings.HasPrefix(loc.URL, "git@")
}

// Probe: size and modification time avoid hashing local archives before each resolution.
func (FileSource) Probe(_ context.Context, loc Locator) (Probe, error) {
	absPath, err := resolveLocalPath(loc.URL)
	if err != nil {
		return Probe{}, err
	}

	identity := absPath
	if info, err := os.Stat(absPath); err == nil && !info.IsDir() {
		identity = fmt.Sprintf("%s|%d|%d", absPath, info.Size(), info.ModTime().UnixNano())
	}
	sum := sha256.Sum256([]byte(identity))

	return Probe{
		Identity: "local-path:" + hex.EncodeToString(sum[:]),
		Local:    absPath,
	}, nil
}

func (FileSource) Fetch(_ context.Context, loc Locator, _ string) error {
	return fmt.Errorf("local resource %q is used in place, not fetched", loc.URL)
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
