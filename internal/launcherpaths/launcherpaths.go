// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package launcherpaths resolves the launcher's own directory location. It
// has no dependency on internal/config or internal/presets, so any package
// that needs this alone (internal/runtimeartifacts, for one) can depend on
// it directly without pulling in either — and, in particular, without
// risking an import cycle should internal/presets ever depend on
// internal/runtimeartifacts.
package launcherpaths

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	rootDirName     = ".exasol"
	personalDirName = "personal"
)

// RootDirPath returns the launcher-owned directory under the current user's
// home directory.
func RootDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for launcher root directory: %w", err)
	}

	return DirPath(home), nil
}

// DirPath returns the launcher-owned directory below baseDir.
func DirPath(baseDir string) string {
	return filepath.Join(baseDir, rootDirName, personalDirName)
}
