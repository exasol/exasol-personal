// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"
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
