// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestIsVersionUpdateAvailable_UsesSemanticVersionOrdering(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		currentVersion string
		latestVersion  string
		expected       bool
	}{
		{
			name:           "older official release is not newer than newer release candidate",
			currentVersion: "2.0.0-rc1",
			latestVersion:  "1.4.1",
			expected:       false,
		},
		{
			name:           "equal versions are not updates",
			currentVersion: "1.4.1",
			latestVersion:  "1.4.1",
			expected:       false,
		},
		{
			name:           "newer patch version is an update",
			currentVersion: "1.4.0",
			latestVersion:  "1.4.1",
			expected:       true,
		},
		{
			name:           "final release is newer than its release candidate",
			currentVersion: "2.0.0-rc1",
			latestVersion:  "2.0.0",
			expected:       true,
		},
		{
			name:           "release candidate is newer than older final release",
			currentVersion: "1.4.1",
			latestVersion:  "2.0.0-rc1",
			expected:       true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given: current and latest launcher versions.
			// When: update availability is evaluated.
			actual, err := IsVersionUpdateAvailable(test.currentVersion, test.latestVersion)
			// Then: semantic version precedence decides whether an update is available.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual != test.expected {
				t.Fatalf("expected update availability %t, got %t", test.expected, actual)
			}
		})
	}
}

func TestIsVersionUpdateAvailable_RejectsInvalidVersionData(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		currentVersion string
		latestVersion  string
		expectedError  string
	}{
		{
			name:           "invalid current version",
			currentVersion: "not-a-version",
			latestVersion:  "1.4.1",
			expectedError:  "failed to parse current launcher version",
		},
		{
			name:           "missing latest version",
			currentVersion: "1.4.1",
			latestVersion:  "",
			expectedError:  "failed to parse latest launcher version",
		},
		{
			name:           "invalid latest version",
			currentVersion: "1.4.1",
			latestVersion:  "latest",
			expectedError:  "failed to parse latest launcher version",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given: invalid version input.
			// When: update availability is evaluated.
			updateAvailable, err := IsVersionUpdateAvailable(
				test.currentVersion,
				test.latestVersion,
			)

			// Then: no update is reported and the parse problem is surfaced.
			if err == nil {
				t.Fatal("expected error")
			}
			if updateAvailable {
				t.Fatal("expected invalid version data to not report an update")
			}
			if !strings.Contains(err.Error(), test.expectedError) {
				t.Fatalf("expected error to contain %q, got %q", test.expectedError, err)
			}
		})
	}
}

func TestParseVersionCheckResponse_ParsesValidJSON(t *testing.T) {
	t.Parallel()

	// Given a valid version check response body.
	body := strings.NewReader(`{"latestVersion":{"version":"1.2.3","filename":"exasol"}}`)

	// When it is parsed.
	response, err := parseVersionCheckResponse(body)
	// Then it succeeds and returns the parsed version.
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if response.LatestVersion.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %q", response.LatestVersion.Version)
	}
}

func TestParseVersionCheckResponse_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	// When an invalid JSON body is parsed.
	_, err := parseVersionCheckResponse(strings.NewReader("{not json"))
	// Then it returns an error.
	if err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

// version_check_details_test.go already covers ClusterIdentity resolution;
// this only adds the OS/Architecture/Category mapping, which isn't covered there.
func TestGetVersionCheckDetails_MapsOSAndArchitecture(t *testing.T) {
	t.Parallel()

	// Given no deployment context.
	var noDeployment config.DeploymentDir

	// When version check details are gathered.
	details := GetVersionCheckDetails(noDeployment)

	// Then the category, OS, and architecture are populated.
	if details.Category != VersionCheckCategory {
		t.Fatalf("expected category %q, got %q", VersionCheckCategory, details.Category)
	}
	if details.OperatingSystem == "" || details.Architecture == "" {
		t.Fatalf("expected non-empty OS/architecture, got %+v", details)
	}
}

func TestCheckLatestVersion_SendsExpectedQueryParamsAndParsesResponse(t *testing.T) {
	t.Parallel()

	// Given a test server that records the requested query and returns a version.
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VersionCheckResponse{
			LatestVersion: LatestVersionInfo{Version: "9.9.9"},
		})
	}))
	defer server.Close()

	details := &VersionCheckDetails{
		OperatingSystem: "Linux",
		Architecture:    "x86_64",
		Category:        VersionCheckCategory,
		ClusterIdentity: "cluster-xyz",
		URL:             server.URL,
	}

	// When the latest version is checked.
	response, err := CheckLatestVersion(context.Background(), details, "1.0.0")
	// Then it succeeds, parses the response, and sends the expected query parameters.
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if response.LatestVersion.Version != "9.9.9" {
		t.Fatalf("expected version 9.9.9, got %q", response.LatestVersion.Version)
	}

	query, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("failed to parse recorded query %q: %v", gotQuery, err)
	}
	for param, want := range map[string]string{
		"category":        VersionCheckCategory,
		"operatingSystem": "Linux",
		"architecture":    "x86_64",
		"version":         "1.0.0",
		"identity":        "cluster-xyz",
	} {
		if got := query.Get(param); got != want {
			t.Fatalf("expected %s=%q, got %q (full query %q)", param, want, got, gotQuery)
		}
	}
}

func TestCheckLatestVersion_NonOKStatusReturnsError(t *testing.T) {
	t.Parallel()

	// Given a test server that returns a non-OK status with a body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	details := &VersionCheckDetails{URL: server.URL}

	// When the latest version is checked.
	_, err := CheckLatestVersion(context.Background(), details, "1.0.0")

	// Then it returns an error mentioning the response body.
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected an error mentioning the response body, got %v", err)
	}
}

func TestFetchLatestVersion_UsesEnvOverrideURL(t *testing.T) {
	// Given a test server and an env override URL pointing to it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(VersionCheckResponse{
			LatestVersion: LatestVersionInfo{Version: "5.0.0"},
		})
	}))
	defer server.Close()
	t.Setenv(VersionCheckURLEnvVar, server.URL)

	var noDeployment config.DeploymentDir

	// When the latest version is fetched.
	response, err := FetchLatestVersion(context.Background(), "1.0.0", noDeployment)
	// Then it succeeds using the overridden URL.
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if response.LatestVersion.Version != "5.0.0" {
		t.Fatalf("expected version 5.0.0, got %q", response.LatestVersion.Version)
	}
}

func TestCheckLatestVersionUpdate_ReportsWhenUpdateIsAvailable(t *testing.T) {
	// Given a test server reporting a newer version, reachable via the env override.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(VersionCheckResponse{
			LatestVersion: LatestVersionInfo{Version: "2.0.0"},
		})
	}))
	defer server.Close()
	t.Setenv(VersionCheckURLEnvVar, server.URL)

	var noDeployment config.DeploymentDir

	// When update availability is checked.
	available, latest, err := CheckLatestVersionUpdate(
		context.Background(), "1.0.0", noDeployment,
	)
	// Then it reports the update as available with the latest version.
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !available || latest != "2.0.0" {
		t.Fatalf("expected an available update to 2.0.0, got available=%v latest=%q",
			available, latest)
	}
}

func TestMustDoVersionCheck_RespectsDisabledFlag(t *testing.T) {
	t.Parallel()

	// Given version checking disabled in state.
	state := &config.ExasolPersonalState{VersionCheckEnabled: false}

	// Then: checking whether a version check must run is skipped.
	if MustDoVersionCheck(state) {
		t.Fatal("expected version check to be skipped when disabled")
	}
}

func TestMustDoVersionCheck_SkipsWithinThrottleWindow(t *testing.T) {
	t.Parallel()

	// Given a check that ran just now.
	state := &config.ExasolPersonalState{
		VersionCheckEnabled: true,
		LastVersionCheck:    time.Now(),
	}

	// Then: checking whether a version check must run is throttled shortly after the last check.
	if MustDoVersionCheck(state) {
		t.Fatal("expected version check to be throttled shortly after the last check")
	}
}

func TestPerformSilentVersionCheck_DisabledSkipsCheck(t *testing.T) {
	t.Parallel()

	// Given a deployment with version checking disabled.
	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{VersionCheckEnabled: false}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to seed launcher state: %v", err)
	}

	// When a silent version check is performed.
	result, err := PerformSilentVersionCheck(context.Background(), deployment, "1.0.0")
	// Then it succeeds without performing the check.
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Checked {
		t.Fatalf("expected the check to be skipped when disabled, got %+v", result)
	}
}

func TestPerformSilentVersionCheck_PerformsCheckAndPersistsResult(t *testing.T) {
	// Given a test server reporting a newer version and a deployment whose throttle
	// window has expired.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(VersionCheckResponse{
			LatestVersion: LatestVersionInfo{Version: "2.0.0"},
		})
	}))
	defer server.Close()
	t.Setenv(VersionCheckURLEnvVar, server.URL)

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		VersionCheckEnabled: true,
		LastVersionCheck:    time.Now().Add(-48 * time.Hour),
	}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to seed launcher state: %v", err)
	}

	// When a silent version check is performed.
	result, err := PerformSilentVersionCheck(context.Background(), deployment, "1.0.0")
	// Then it performs the check, reports the available update, and persists the
	// new check time.
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Checked || !result.UpdateAvailable || result.LatestVersion != "2.0.0" {
		t.Fatalf("expected a completed check reporting an available update, got %+v", result)
	}

	persisted, readErr := config.ReadExasolPersonalState(deployment)
	if readErr != nil {
		t.Fatalf("failed to read persisted state: %v", readErr)
	}
	if !persisted.LastVersionCheck.After(state.LastVersionCheck) {
		t.Fatalf("expected LastVersionCheck to be updated, got %v", persisted.LastVersionCheck)
	}
}

func TestPerformSilentVersionCheck_MissingStateReturnsError(t *testing.T) {
	t.Parallel()

	// Given a deployment with no persisted launcher state.
	deployment := config.NewDeploymentDir(t.TempDir())

	// When a silent version check is performed.
	result, err := PerformSilentVersionCheck(context.Background(), deployment, "1.0.0")
	// Then it returns an error and reports no check performed.
	if err == nil {
		t.Fatal("expected an error when no launcher state has been persisted")
	}
	if result.Checked {
		t.Fatalf("expected no check to be reported on error, got %+v", result)
	}
}

func TestMustDoVersionCheck_RunsAfterThrottleWindowExpires(t *testing.T) {
	t.Parallel()

	// Given a check that ran well outside the throttle window.
	state := &config.ExasolPersonalState{
		VersionCheckEnabled: true,
		LastVersionCheck:    time.Now().Add(-48 * time.Hour),
	}

	// Then: a version check must run once the throttle window has passed.
	if !MustDoVersionCheck(state) {
		t.Fatal("expected version check to run once the throttle window has passed")
	}
}
