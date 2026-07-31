// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

const (
	slcStagingDirName = "slc-packages"
	slcStatusFileName = "slc-status.json"

	slcStagingDirMode  = 0o750
	slcStagingFileMode = 0o640

	slcDownloadPrefix = "incoming-"
	slcDownloadSuffix = ".part"
)

const (
	slcStatePresent  = "present"
	slcStateImported = "imported"
	slcStatePulled   = "pulled"
)

type slcImageStatus struct {
	Image string `json:"image"`
	State string `json:"state"`
}

//nolint:tagliatelle // JSON key mirrors the "SLC" domain abbreviation.
type slcStatusReport struct {
	SLC []slcImageStatus `json:"slc"`
}

// Only the shared directory is reachable by the guest that imports the package.
func customSLCStagingDir(deployment config.DeploymentDir) string {
	return filepath.Join(localruntime.NewPaths(deployment).SharedDir, slcStagingDirName)
}

func customSLCStatusPath(deployment config.DeploymentDir) string {
	return filepath.Join(localruntime.NewPaths(deployment).SharedDir, slcStatusFileName)
}

// Downloads write here directly: rename is only atomic within one filesystem.
func newCustomSLCStagingFile(deployment config.DeploymentDir) (*os.File, error) {
	dir := customSLCStagingDir(deployment)
	if err := os.MkdirAll(dir, slcStagingDirMode); err != nil {
		return nil, fmt.Errorf("failed to create the custom SLC staging directory: %w", err)
	}
	// The deployment lock is held here, so any partial download present is stale.
	discardStaleCustomSLCDownloads(dir)

	temp, err := os.CreateTemp(dir, slcDownloadPrefix+"*"+slcDownloadSuffix)
	if err != nil {
		return nil, fmt.Errorf("failed to stage the custom SLC container: %w", err)
	}

	return temp, nil
}

func discardStaleCustomSLCDownloads(dir string) {
	stale, err := filepath.Glob(filepath.Join(dir, slcDownloadPrefix+"*"+slcDownloadSuffix))
	if err != nil {
		return
	}
	for _, path := range stale {
		if err := os.Remove(path); err != nil {
			slog.Warn("failed to remove a stale custom SLC download", "path", path, "error", err)
		}
	}
}

// Renamed into place so a start racing an interrupted stage cannot import a partial package.
func promoteCustomSLCPackage(deployment config.DeploymentDir, tempPath, name string) error {
	if err := os.Chmod(tempPath, slcStagingFileMode); err != nil {
		return fmt.Errorf("failed to stage the custom SLC container: %w", err)
	}
	target := filepath.Join(customSLCStagingDir(deployment), name)
	if err := os.Rename(tempPath, target); err != nil {
		return fmt.Errorf("failed to stage the custom SLC container: %w", err)
	}

	return nil
}

func stageCustomSLCPackage(deployment config.DeploymentDir, name string, tarball io.Reader) error {
	temp, err := newCustomSLCStagingFile(deployment)
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if _, err := io.Copy(temp, tarball); err != nil {
		_ = temp.Close()

		return fmt.Errorf("failed to stage the custom SLC container: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to stage the custom SLC container: %w", err)
	}

	return promoteCustomSLCPackage(deployment, tempPath, name)
}

func removeCustomSLCPackage(deployment config.DeploymentDir, name string) error {
	if name == "" {
		return nil
	}
	err := os.Remove(filepath.Join(customSLCStagingDir(deployment), name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func readSLCImageStates(deployment config.DeploymentDir) (map[string]string, error) {
	path := customSLCStatusPath(deployment)
	content, err := os.ReadFile(path) //nolint:gosec // path is launcher-owned
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}

		return nil, err
	}

	var report slcStatusReport
	if err := json.Unmarshal(content, &report); err != nil {
		return nil, fmt.Errorf("failed to parse the local SLC status report %s: %w", path, err)
	}

	states := make(map[string]string, len(report.SLC))
	for _, status := range report.SLC {
		states[status.Image] = status.State
	}

	return states, nil
}

func customSLCImageAvailable(states map[string]string, image string) bool {
	switch states[image] {
	case slcStatePresent, slcStateImported, slcStatePulled:
		return true
	default:
		return false
	}
}
