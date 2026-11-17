// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/exasol/exasol-personal/internal/launcherpaths"
)

func resolveManagedDeploymentDir(name string) (string, error) {
	newPath, err := launcherpaths.NamedDeploymentDirPath(name)
	if err != nil {
		return "", err
	}
	if exists, err := managedPathExists(newPath); err != nil {
		return "", fmt.Errorf("inspect deployment path %s: %w", newPath, err)
	} else if exists {
		return newPath, nil
	}

	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return "", err
	}
	legacyPath := filepath.Join(legacyRoot, name)
	if exists, err := managedPathExists(legacyPath); err != nil {
		return "", fmt.Errorf("inspect deployment path %s: %w", legacyPath, err)
	} else if exists {
		return legacyPath, nil
	}

	return newPath, nil
}

func managedPathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func isLegacyManagedDeploymentDir(path string) bool {
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	return err == nil && sameLocation(filepath.Dir(path), legacyRoot)
}

// followMigratedDeployment keeps a legacy managed deployment path working after
// startup migration moved that deployment, including on later invocations.
func followMigratedDeployment(path string) (string, bool) {
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil || !sameLocation(filepath.Dir(abs), legacyRoot) {
		return "", false
	}
	if exists, err := pathExists(abs); err != nil || exists {
		return "", false
	}
	moved, err := launcherpaths.NamedDeploymentDirPath(filepath.Base(abs))
	if err != nil {
		return "", false
	}
	if exists, err := pathExists(moved); err != nil || !exists {
		return "", false
	}

	return moved, true
}

// caseInsensitivePaths matches the default file systems of these platforms.
var caseInsensitivePaths = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// sameLocation tolerates symbolic links and letter case on case-insensitive
// file systems. Either path may no longer exist after migration, so only
// their nearest existing ancestors are compared as files.
func sameLocation(a, b string) bool {
	aBase, aRest := splitExisting(a)
	bBase, bRest := splitExisting(b)
	if len(aRest) != len(bRest) {
		return false
	}
	for i := range aRest {
		if aRest[i] != bRest[i] &&
			(!caseInsensitivePaths || !strings.EqualFold(aRest[i], bRest[i])) {
			return false
		}
	}
	aInfo, aErr := os.Stat(aBase)
	bInfo, bErr := os.Stat(bBase)

	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}

// splitExisting returns the nearest existing ancestor of path and the names
// below it.
func splitExisting(path string) (string, []string) {
	var rest []string
	for {
		if _, err := os.Stat(path); err == nil {
			return path, rest
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path, rest
		}
		rest = append([]string{filepath.Base(path)}, rest...)
		path = parent
	}
}

// isWithin reports whether path is dir or lies below it.
func isWithin(path, dir string) bool {
	for {
		if sameLocation(path, dir) {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}

func managedDeploymentEntries() ([]deploymentListEntry, error) {
	newRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		return nil, err
	}
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		return nil, err
	}
	newDirs, err := readManagedDeploymentDirectories(newRoot)
	if err != nil {
		return nil, err
	}
	legacyDirs, err := readManagedDeploymentDirectories(legacyRoot)
	if err != nil {
		return nil, err
	}

	entries := make([]deploymentListEntry, 0, len(newDirs)+len(legacyDirs))
	for _, dirEntry := range newDirs {
		entries = append(entries, deploymentListEntry{
			Name: dirEntry.Name(),
			Path: filepath.Join(newRoot, dirEntry.Name()),
		})
	}
	for _, dirEntry := range legacyDirs {
		newPath := filepath.Join(newRoot, dirEntry.Name())
		if exists, err := managedPathExists(newPath); err != nil {
			return nil, fmt.Errorf("inspect deployment path %s: %w", newPath, err)
		} else if exists {
			continue
		}
		entries = append(entries, deploymentListEntry{
			Name: dirEntry.Name(),
			Path: filepath.Join(legacyRoot, dirEntry.Name()),
		})
	}

	return entries, nil
}
