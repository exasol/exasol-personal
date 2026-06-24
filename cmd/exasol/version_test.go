// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
)

func TestFormatCurrentVersionText_ReportsText(t *testing.T) {
	t.Parallel()

	// Given: a current launcher version.
	// When: text output is formatted.
	actual := formatCurrentVersionText("2.0.0-rc1")

	// Then: the version remains plain text.
	if actual != "2.0.0-rc1" {
		t.Fatalf("expected text output %q, got %q", "2.0.0-rc1", actual)
	}
}

func TestFormatCurrentVersionJSON_ReportsStructuredJSON(t *testing.T) {
	t.Parallel()

	// Given: a current launcher version.
	// When: JSON output is formatted.
	actual, err := formatCurrentVersionJSON("2.0.0-rc1")
	// Then: the version is returned as structured JSON.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(actual), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", actual, err)
	}
	if parsed.Version != "2.0.0-rc1" {
		t.Fatalf("expected JSON version %q, got %q", "2.0.0-rc1", parsed.Version)
	}
}

func TestFormatLatestVersionText_ReportsNewerEqualAndOlderVersions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		currentVersion string
		latestVersion  string
		expected       []string
		unexpected     []string
	}{
		{
			name:           "reported version is newer",
			currentVersion: "1.4.0",
			latestVersion:  "1.4.1",
			expected: []string{
				"The latest official version of Exasol Personal available is: 1.4.1",
				"(you are using 1.4.0)",
				"  Version: 1.4.1",
				"  Download URL: https://example.com/exasol",
			},
			unexpected: []string{"No newer official version is available"},
		},
		{
			name:           "reported version is equal",
			currentVersion: "1.4.1",
			latestVersion:  "1.4.1",
			expected: []string{
				"You are using the latest version of Exasol Personal (1.4.1).",
			},
			unexpected: []string{"Download URL", "No newer official version is available"},
		},
		{
			name:           "reported version is older",
			currentVersion: "2.0.0-rc1",
			latestVersion:  "1.4.1",
			expected: []string{
				"The latest official version of Exasol Personal available is: 1.4.1",
				"(you are using 2.0.0-rc1)",
				"No newer official version is available.",
			},
			unexpected: []string{"Download URL"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given: a latest-version response.
			response := latestVersionResponse(test.latestVersion)
			// When: the response is rendered as user-facing text.
			actual, err := formatLatestVersionText(test.currentVersion, response)
			// Then: the output describes whether the reported version is newer.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, expected := range test.expected {
				if !strings.Contains(actual, expected) {
					t.Fatalf("expected output to contain %q, got %q", expected, actual)
				}
			}
			for _, unexpected := range test.unexpected {
				if strings.Contains(actual, unexpected) {
					t.Fatalf("expected output to not contain %q, got %q", unexpected, actual)
				}
			}
		})
	}
}

func TestFormatLatestVersionText_RejectsInvalidVersionData(t *testing.T) {
	t.Parallel()

	// Given: an invalid latest-version response.
	response := latestVersionResponse("")

	// When: the response is rendered as user-facing text.
	_, err := formatLatestVersionText("1.4.1", response)

	// Then: the invalid response is surfaced as an error.
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to parse latest launcher version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVersionCmdQueuesPrimaryOutputForTerminalFlush(t *testing.T) {
	t.Parallel()

	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given: the version command emits primary output through the terminal queue.
	addTerminalOutput("1.4.1")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// When: queued terminal messages are flushed.
	writeTerminalMessages(&stdout, &stderr)

	// Then: primary command output goes to stdout and notices do not pollute it.
	if stdout.String() != "1.4.1\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func latestVersionResponse(version string) *deploy.VersionCheckResponse {
	return &deploy.VersionCheckResponse{
		LatestVersion: deploy.LatestVersionInfo{
			Version:         version,
			Filename:        "exasol",
			URL:             "https://example.com/exasol",
			Size:            42,
			SHA256:          "abc123",
			OperatingSystem: "Linux",
			Architecture:    "x86_64",
		},
	}
}
