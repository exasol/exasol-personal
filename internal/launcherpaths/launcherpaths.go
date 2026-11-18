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
	rootDirName = ".exasol"

	namespaceDirName         = "exasol"
	appDirName               = "launcher"
	deploymentsDirName       = "deployments"
	defaultDeploymentDirName = "default"
	historyDirName           = "history"
	locksDirName             = "locks"
	historyFileName          = "exasol_history"
	resourceConfigFileName   = "resources.yaml"
	resourceCacheDirName     = "resources"
)

func DeploymentsRootPath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, deploymentsDirName), nil
}

func DefaultDeploymentDirPath() (string, error) {
	return NamedDeploymentDirPath(defaultDeploymentDirName)
}

// NamedDeploymentDirPath returns the launcher-managed deployment directory
// path for name, in the same parent directory as the default deployment
// directory. Callers are responsible for validating name is safe to use as a
// literal directory name (see DeploymentNameVar in cmd/exasol).
func NamedDeploymentDirPath(name string) (string, error) {
	root, err := DeploymentsRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, name), nil
}

func HistoryFilePath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, historyDirName, historyFileName), nil
}

// LocksRootPath returns where the launcher keeps lock files for directories
// that cannot hold their own.
func LocksRootPath() (string, error) {
	root, err := durableRootPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, locksDirName), nil
}

func ResourceConfigFilePath() (string, error) {
	return resourceConfigFilePathForOS(runtime.GOOS)
}

func resourceConfigFilePathForOS(goos string) (string, error) {
	if goos == "darwin" {
		root, err := durableRootPathForOS(goos)
		if err != nil {
			return "", err
		}

		return filepath.Join(root, resourceConfigFileName), nil
	}

	return namespacedPath(os.UserConfigDir, resourceConfigFileName)
}

func CacheRootPath() (string, error) {
	return namespacedPath(os.UserCacheDir, resourceCacheDirName)
}

func namespacedPath(baseDir func() (string, error), name string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", fmt.Errorf("resolve base directory for launcher %s: %w", name, err)
	}

	return filepath.Join(launcherNamespaceRoot(base), name), nil
}

// Windows uses %LOCALAPPDATA% (os.UserCacheDir) because deployments hold
// machine-bound state and %APPDATA% roams with the user profile.
func durableRootPath() (string, error) {
	return durableRootPathForOS(runtime.GOOS)
}

func durableRootPathForOS(goos string) (string, error) {
	if goos == "linux" || goos == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for launcher durable root: %w", err)
		}

		return filepath.Join(home, rootDirName, appDirName), nil
	}

	baseDir := os.UserConfigDir
	if goos == "windows" {
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
