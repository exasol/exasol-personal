// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

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
