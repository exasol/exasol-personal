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

func TestFormatStatusText(t *testing.T) {
	t.Parallel()

	// Given
	status := deploy.StatusOutput{
		DeploymentDir: "/deployment",
		Status:        deploy.StatusNotInitialized,
		Message:       "create one",
	}

	// When
	output := formatStatusText(status)

	// Then
	for _, expected := range []string{
		"Deployment directory: /deployment",
		"Status: not_initialized",
		"Message: create one",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected status text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestFormatStatusJSON(t *testing.T) {
	t.Parallel()

	// Given
	status := deploy.StatusOutput{
		DeploymentDir: "/deployment",
		Status:        deploy.StatusDatabaseReady,
	}

	// When
	output, err := formatStatusJSON(status)
	// Then
	if err != nil {
		t.Fatalf("expected status JSON to render: %v", err)
	}
	var decoded deploy.StatusOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	if decoded.DeploymentDir != status.DeploymentDir {
		t.Fatalf("expected deployment dir %q, got %q", status.DeploymentDir, decoded.DeploymentDir)
	}
	if decoded.Status != status.Status {
		t.Fatalf("expected status %q, got %q", status.Status, decoded.Status)
	}
}

//nolint:paralleltest // Uses package-global terminal message queues.
func TestStatusOutputUsesQueuedTerminalOutput(t *testing.T) {
	// Given
	resetTerminalMessages()
	defer resetTerminalMessages()
	addTerminalOutput(formatStatusText(deploy.StatusOutput{
		DeploymentDir: "/deployment",
		Status:        deploy.StatusInitialized,
	}))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// When
	writeTerminalMessages(&stdout, &stderr, true)

	// Then
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Status: initialized\n") {
		t.Fatalf("expected status output on stdout, got %q", stdout.String())
	}
}
