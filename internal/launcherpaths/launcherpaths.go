// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package launcherpaths resolves every path the launcher owns outside a
// deployment directory. It depends on no other internal package, so any
// package can resolve these paths without risking an import cycle.
package launcherpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	rootDirName     = ".exasol"
	personalDirName = "personal"

	namespaceDirName       = "exasol"
	appDirName             = "launcher"
	deploymentsDirName     = "deployments"
	historyDirName         = "history"
	historyFileName        = "exasol_history"
	resourceConfigFileName = "resources.yaml"
	resourceCacheDirName   = "resources"
	migrationMarkerName    = ".migrated"

	legacyResourceConfigFileName = "runtime-artifacts.yaml"
	legacyResourceCacheDirName   = "runtime-artifacts"
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

func DeploymentsRootPath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, deploymentsDirName), nil
}

func HistoryFilePath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, historyDirName, historyFileName), nil
}

func ResourceConfigFilePath() (string, error) {
	return namespacedPath(os.UserConfigDir, resourceConfigFileName)
}

func CacheRootPath() (string, error) {
	return namespacedPath(os.UserCacheDir, resourceCacheDirName)
}

func MigrationMarkerPath() (string, error) {
	return namespacedPath(os.UserConfigDir, migrationMarkerName)
}

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

	return filepath.Join(DirPath(base), name), nil
}

func namespacedPath(baseDir func() (string, error), name string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", fmt.Errorf("resolve base directory for launcher %s: %w", name, err)
	}

	return filepath.Join(launcherNamespaceRoot(base), name), nil
}

// Linux keeps durable data in a dotfile location separate from config.
// Windows uses %LOCALAPPDATA% (os.UserCacheDir) because deployments hold
// machine-bound state and %APPDATA% roams with the user profile.
func durableRootPath() (string, error) {
	if runtime.GOOS == "linux" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for launcher durable root: %w", err)
		}

		return filepath.Join(home, rootDirName, appDirName), nil
	}

	baseDir := os.UserConfigDir
	if runtime.GOOS == "windows" {
		baseDir = os.UserCacheDir
	}
	base, err := baseDir()
	if err != nil {
		return "", fmt.Errorf("resolve base directory for launcher durable root: %w", err)
	}

	return launcherNamespaceRoot(base), nil
}

func launcherNamespaceRoot(base string) string {
	return filepath.Join(base, namespaceDirName, appDirName)
}
