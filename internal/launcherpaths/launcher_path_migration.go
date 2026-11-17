// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package launcherpaths

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	personalDirName              = "personal"
	legacyResourceConfigFileName = "runtime-artifacts.yaml"
	legacyResourceCacheDirName   = "runtime-artifacts"
)

// The legacy paths below exist only so startup migration can move or delete
// what earlier launcher versions left there.

func LegacyDeploymentsRootPath() (string, error) {
	return legacyPath(os.UserHomeDir, deploymentsDirName)
}

func LegacyHistoryFilePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve base directory for legacy %s: %w", historyFileName, err)
	}

	return filepath.Join(cacheDir, historyFileName), nil
}

func LegacyResourceConfigFilePath() (string, error) {
	return legacyPath(os.UserHomeDir, legacyResourceConfigFileName)
}

func LegacyCacheRootPath() (string, error) {
	return legacyPath(os.UserCacheDir, legacyResourceCacheDirName)
}

func legacyPath(baseDir func() (string, error), name string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", fmt.Errorf("resolve base directory for legacy %s: %w", name, err)
	}

	return filepath.Join(base, rootDirName, personalDirName, name), nil
}
