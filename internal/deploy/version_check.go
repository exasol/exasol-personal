// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/version_check"
)

const (
	VersionCheckCategory    = "Exasol Personal"
	VersionCheckLockTimeout = 250 * time.Millisecond
)

// GetVersionCheckDetails returns platform-specific version check details.
// The version check URL can be overridden using the EXASOL_VERSION_CHECK_URL environment variable.
// If deploymentDir is provided and contains a state file, uses its persisted ClusterIdentity.
// If no valid state file exists (e.g. when called outside a deployment directory),
// ClusterIdentity is left empty.
func GetVersionCheckDetails(deployment config.DeploymentDir) *version_check.VersionCheckDetails {
	operatingSystem := runtime.GOOS
	arch := runtime.GOARCH

	switch operatingSystem {
	case "linux":
		operatingSystem = "Linux"
	case "darwin":
		operatingSystem = "MacOS"
	case "windows":
		operatingSystem = "Windows"
	default:
		// Keep the original value for unknown systems
	}

	if arch == "amd64" {
		arch = "x86_64"
	}

	// Determine cluster identity.
	clusterIdentity := ""
	if deployment.Root() != "" {
		// Prefer the launcher-governed, persisted identity.
		if exasolState, err := config.ReadExasolPersonalState(deployment); err == nil {
			if v := strings.TrimSpace(exasolState.ClusterIdentity); v != "" {
				clusterIdentity = v
				slog.Debug(
					"using persisted cluster identity",
					"clusterIdentity",
					clusterIdentity,
				)
			}
		}
	}

	return &version_check.VersionCheckDetails{
		OperatingSystem: operatingSystem,
		Architecture:    arch,
		Category:        VersionCheckCategory,
		ClusterIdentity: clusterIdentity,
		URL:             version_check.GetVersionCheckURL(),
	}
}

// FetchLatestVersion retrieves version information for the current platform.
func FetchLatestVersion(
	ctx context.Context,
	currentVersion string,
	deployment config.DeploymentDir,
) (*version_check.VersionCheckResponse, error) {
	details := GetVersionCheckDetails(deployment)
	return version_check.CheckLatestVersion(ctx, details, currentVersion)
}

func MustDoVersionCheck(exasolState *config.ExasolPersonalState) bool {
	if !exasolState.VersionCheckEnabled {
		slog.Debug("skipped version check because version checks are disabled.")
		return false
	}

	const hoursUntilNextCheck = 24
	currentTime := time.Now()
	nextCheckTime := exasolState.LastVersionCheck.Add(hoursUntilNextCheck * time.Hour)

	slog.Debug("checking last version check time",
		"currentTime", currentTime.Format(time.RFC3339),
		"lastTime", exasolState.LastVersionCheck,
		"nextTime", nextCheckTime)

	if currentTime.Before(nextCheckTime) {
		slog.Debug("skipping version check due to previous check within 24 hours")
		return false
	}

	return true
}

// CheckLatestVersionUpdate checks if an update is available.
func CheckLatestVersionUpdate(
	ctx context.Context,
	currentVersion string,
	deployment config.DeploymentDir,
) (bool, string, error) {
	response, err := FetchLatestVersion(ctx, currentVersion, deployment)
	if err != nil {
		return false, "", err
	}
	latest := response.LatestVersion.Version
	available, err := version_check.IsVersionUpdateAvailable(currentVersion, latest)
	if err != nil {
		return false, latest, err
	}

	return available, latest, nil
}

// PerformSilentVersionCheck runs a guarded version check and updates state.
// It returns whether a check was performed and whether an update is available.
func PerformSilentVersionCheck(
	ctx context.Context,
	deployment config.DeploymentDir,
	currentVersion string,
) (version_check.SilentVersionCheckResult, error) {
	slog.Debug("begin version update check")

	result := version_check.SilentVersionCheckResult{}

	lockCtx, cancel := context.WithTimeout(ctx, VersionCheckLockTimeout)
	defer cancel()
	err := withDeploymentExclusiveLock(
		lockCtx,
		deployment,
		func(deployment config.DeploymentDir) error {
			exasolState, readErr := config.ReadExasolPersonalState(deployment)
			if readErr != nil {
				return readErr
			}

			if !MustDoVersionCheck(exasolState) {
				slog.Debug("launcher version update check disabled")
				return nil
			}

			available, latest, checkErr := CheckLatestVersionUpdate(ctx, currentVersion, deployment)
			defer func() {
				// Treat all attempts as a check for throttling purposes.
				exasolState.LastVersionCheck = time.Now()
				_ = config.WriteExasolPersonalState(exasolState, deployment)
			}()

			if checkErr != nil {
				slog.Debug("launcher version update check failed")
				return checkErr
			}

			result.Checked = true
			result.UpdateAvailable = available
			result.LatestVersion = latest

			return nil
		},
	)
	if err != nil {
		// If the state is locked by another process (or acquisition timed out), silently skip.
		if errors.Is(err, ErrDeploymentDirectoryLocked) || errors.Is(err, context.Canceled) {
			slog.Debug("launcher version update check state not available")
			return result, nil
		}

		return result, err
	}

	return result, nil
}
