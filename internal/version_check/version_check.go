// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package version_check

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/blang/semver/v4"
)

const (
	VersionCheckURLEnvVar = "EXASOL_VERSION_CHECK_URL"
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

// IsVersionUpdateAvailable reports whether latestVersion is newer than currentVersion.
func IsVersionUpdateAvailable(currentVersion, latestVersion string) (bool, error) {
	current, err := semver.Parse(currentVersion)
	if err != nil {
		return false, fmt.Errorf(
			"failed to parse current launcher version %q: %w",
			currentVersion,
			err,
		)
	}
	latest, err := semver.Parse(latestVersion)
	if err != nil {
		return false, fmt.Errorf(
			"failed to parse latest launcher version %q: %w",
			latestVersion,
			err,
		)
	}

	return latest.GT(current), nil
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
		params.Add("identity", details.ClusterIdentity)
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
