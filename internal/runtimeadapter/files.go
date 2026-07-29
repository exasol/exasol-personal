// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	privateDirMode  = 0o750
	privateFileMode = 0o600
	executableMode  = 0o700
)

type workloadImageStage int

const (
	stageManifestOnly workloadImageStage = iota
	stageManifestAndImage
)

func stageWorkloadAssets(
	spec WorkloadSpec,
	manifestPath, imagePath string,
	imageStage workloadImageStage,
) error {
	manifest, err := RenderKubeManifest(spec)
	if err != nil {
		return err
	}
	if err := writeAtomic(manifestPath, manifest, privateFileMode); err != nil {
		return err
	}
	if imageStage == stageManifestOnly {
		return nil
	}
	imageArchive, err := spec.ImageBytes()
	if err != nil {
		return err
	}

	return writeAtomicIfChanged(imagePath, imageArchive, privateFileMode)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return fmt.Errorf("failed to create runtime artifact directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary runtime artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf(
			"failed to set runtime artifact permissions: %w",
			errors.Join(err, temporary.Close()),
		)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf(
			"failed to write runtime artifact: %w",
			errors.Join(err, temporary.Close()),
		)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf(
			"failed to sync runtime artifact: %w",
			errors.Join(err, temporary.Close()),
		)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close runtime artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("failed to replace runtime artifact: %w", err)
	}

	return nil
}

func writeAtomicIfChanged(path string, data []byte, mode os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect runtime artifact: %w", err)
	}

	return writeAtomic(path, data, mode)
}

// RemovePersonalData removes an explicitly selected deployment-owned data
// target after validating that no parent or target symlink can escape the
// deployment. Runtime adapters never call this; Personal's top-level destroy
// operation is the sole caller.
func RemovePersonalData(deploymentRoot, dataPath string) error {
	root, err := filepath.Abs(deploymentRoot)
	if err != nil {
		return err
	}
	data, err := filepath.Abs(dataPath)
	if err != nil {
		return err
	}
	expectedRoot := filepath.Join(root, "local", "data")
	if _, err := os.Lstat(data); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to inspect Personal-owned data %s: %w", data, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(expectedRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve Personal-owned data root %s: %w", expectedRoot, err)
	}
	if resolvedRoot != expectedRoot {
		return fmt.Errorf(
			"refusing to remove data through symlinked Personal data root %s",
			expectedRoot,
		)
	}
	resolvedData, err := filepath.EvalSymlinks(data)
	if err != nil {
		return fmt.Errorf("failed to resolve Personal-owned data %s: %w", data, err)
	}
	if resolvedData != data {
		return fmt.Errorf("refusing to remove data through symlink %s", data)
	}
	relative, err := filepath.Rel(expectedRoot, data)
	if err != nil || relative == ".." ||
		(len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove data outside Personal-owned local data: %s", data)
	}
	if err := os.RemoveAll(data); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove Personal-owned data %s: %w", data, err)
	}

	return nil
}
