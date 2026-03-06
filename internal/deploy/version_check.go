// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	DefaultVersionCheckURL = "https://metrics.exasol.com/v1/version-check"
	VersionCheckURLEnvVar  = "EXASOL_VERSION_CHECK_URL"
	VersionCheckCategory   = "Exasol Personal"

	VersionCheckLockTimeout = 250 * time.Millisecond
)

// GetVersionCheckURL resolves the version-check endpoint URL.
// The endpoint can be overridden for tests and controlled environments.
func GetVersionCheckURL() string {
	versionCheckURL := os.Getenv(VersionCheckURLEnvVar)
	if versionCheckURL == "" {
		versionCheckURL = DefaultVersionCheckURL
	}

	return versionCheckURL
}

// VersionCheckDetails contains platform-specific information for version checking.
type VersionCheckDetails struct {
	OperatingSystem string
	Architecture    string
	Category        string
	ClusterIdentity string
	URL             string
}

// GetVersionCheckDetails returns platform-specific version check details.
// The version check URL can be overridden using the EXASOL_VERSION_CHECK_URL environment variable.
// If deploymentDir is provided and contains a state file, uses its persisted ClusterIdentity.
// If no valid state file exists (e.g. when called outside a deployment directory),
// ClusterIdentity is left empty.
func GetVersionCheckDetails(deploymentDir string) *VersionCheckDetails {
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
	if deploymentDir != "" {
		// Prefer the launcher-governed, persisted identity.
		if exasolState, err := config.ReadExasolPersonalState(deploymentDir); err == nil {
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

	return &VersionCheckDetails{
		OperatingSystem: operatingSystem,
		Architecture:    arch,
		Category:        VersionCheckCategory,
		ClusterIdentity: clusterIdentity,
		URL:             GetVersionCheckURL(),
	}
}

// LatestVersionInfo contains information about the latest version.
type LatestVersionInfo struct {
	Version         string `json:"version"`
	Filename        string `json:"filename"`
	URL             string `json:"url"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
	OperatingSystem string `json:"operatingSystem"`
	Architecture    string `json:"architecture"`
}

// VersionCheckResponse represents the response from the version check API.
type VersionCheckResponse struct {
	LatestVersion LatestVersionInfo `json:"latestVersion"`
}

// SilentVersionCheckResult reports whether a version check was performed and its outcome.
type SilentVersionCheckResult struct {
	Checked         bool
	UpdateAvailable bool
	LatestVersion   string
}

func parseVersionCheckResponse(body io.Reader) (*VersionCheckResponse, error) {
	result := &VersionCheckResponse{}

	if err := json.NewDecoder(body).Decode(result); err != nil {
		return nil, fmt.Errorf("failed to parse version check response: %w", err)
	}

	return result, nil
}

const versionCheckTimeout = 3 // seconds

// CheckLatestVersion checks for the latest version from the API.
func CheckLatestVersion(
	ctx context.Context,
	details *VersionCheckDetails,
	currentVersion string,
) (*VersionCheckResponse, error) {
	params := url.Values{}
	params.Add("category", details.Category)
	params.Add("operatingSystem", details.OperatingSystem)
	params.Add("architecture", details.Architecture)
	params.Add("version", currentVersion)
	if strings.TrimSpace(details.ClusterIdentity) != "" {
		params.Add("clusterIdentity", details.ClusterIdentity)
	}

	requestURL := fmt.Sprintf("%s?%s", details.URL, params.Encode())

	slog.Debug("making version check request", "url", requestURL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: versionCheckTimeout * time.Second,
	}

	// Make the request using the provided context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make version check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"version check request failed with status %d: %s",
			resp.StatusCode,
			string(body),
		)
	}

	// Parse the response
	result, err := parseVersionCheckResponse(resp.Body)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// FetchLatestVersion retrieves version information for the current platform.
func FetchLatestVersion(
	ctx context.Context,
	currentVersion string,
	deploymentDir string,
) (*VersionCheckResponse, error) {
	details := GetVersionCheckDetails(deploymentDir)
	return CheckLatestVersion(ctx, details, currentVersion)
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
	deploymentDir string,
) (bool, string, error) {
	response, err := FetchLatestVersion(ctx, currentVersion, deploymentDir)
	if err != nil {
		return false, "", err
	}
	latest := response.LatestVersion.Version
	if latest == "" || latest == currentVersion {
		return false, latest, nil
	}

	return true, latest, nil
}

// PerformSilentVersionCheck runs a guarded version check and updates state.
// It returns whether a check was performed and whether an update is available.
func PerformSilentVersionCheck(
	ctx context.Context,
	deploymentDir string,
	currentVersion string,
) (SilentVersionCheckResult, error) {
	slog.Debug("begin version update check")

	result := SilentVersionCheckResult{}

	lockCtx, cancel := context.WithTimeout(ctx, VersionCheckLockTimeout)
	defer cancel()
	err := withDeploymentExclusiveLock(lockCtx, deploymentDir, func(dir string) error {
		exasolState, readErr := config.ReadExasolPersonalState(dir)
		if readErr != nil {
			return readErr
		}

		if !MustDoVersionCheck(exasolState) {
			slog.Debug("launcher version update check disabled")
			return nil
		}

		available, latest, checkErr := CheckLatestVersionUpdate(ctx, currentVersion, dir)
		defer func() {
			// Treat all attempts as a check for throttling purposes.
			exasolState.LastVersionCheck = time.Now()
			_ = config.WriteExasolPersonalState(exasolState, dir)
		}()

		if checkErr != nil {
			slog.Debug("launcher version update check failed")
			return checkErr
		}

		result.Checked = true
		result.UpdateAvailable = available
		result.LatestVersion = latest

		return nil
	})
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
