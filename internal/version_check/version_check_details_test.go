// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package version_check

import (
	"testing"
)

func TestGetVersionCheckURL_DefaultWhenEnvMissing(t *testing.T) {
	t.Setenv(VersionCheckURLEnvVar, "")

	url := GetVersionCheckURL()
	if url != DefaultVersionCheckURL {
		t.Fatalf("expected default URL %q, got %q", DefaultVersionCheckURL, url)
	}
}

func TestGetVersionCheckURL_UsesEnvOverride(t *testing.T) {
	const expected = "https://example.com/custom-check"
	t.Setenv(VersionCheckURLEnvVar, expected)

	url := GetVersionCheckURL()
	if url != expected {
		t.Fatalf("expected env override URL %q, got %q", expected, url)
	}
}
