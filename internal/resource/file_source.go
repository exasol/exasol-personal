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

func (FileSource) Handles(loc Locator) bool {
	if strings.HasPrefix(loc.URL, FileURLScheme) {
		return true
	}
	// Local filesystem path: no URL scheme and not a git@ remote
	return !strings.Contains(loc.URL, "://") && !strings.HasPrefix(loc.URL, "git@")
}

// Probe reports the content's existing location, so the cache stores no copy
// of it. Extracting it, when asked for, still lands in the cache.
func (FileSource) Probe(_ context.Context, loc Locator) (Probe, error) {
	absPath, err := resolveLocalPath(loc.URL)
	if err != nil {
		return Probe{}, err
	}
	sum := sha256.Sum256([]byte(absPath))

	return Probe{
		Identity: "local-path:" + hex.EncodeToString(sum[:]),
		Local:    absPath,
	}, nil
}

// Fetch is unreachable: Probe always reports a local path, and the manager
// never fetches content it has been told is already in place.
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
