// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/deploy"
)

func TestStatusCommandRegistersDefaultTimeout(t *testing.T) {
	t.Parallel()

	// Given
	flag := statusCmd.Flags().Lookup("timeout")

	// Then
	if flag == nil {
		t.Fatal("expected status command to register --timeout")
	}
	if flag.DefValue != strconv.FormatInt(defaultStatusTimeoutSeconds, 10) {
		t.Fatalf("expected default timeout %d, got %q", defaultStatusTimeoutSeconds, flag.DefValue)
	}
}

func TestContextWithStatusTimeoutUsesSeconds(t *testing.T) {
	t.Parallel()

	// Given
	const timeoutSeconds int64 = 1

	// When
	ctx, cancel, err := contextWithStatusTimeout(context.Background(), timeoutSeconds)
	if err != nil {
		t.Fatalf("expected timeout context, got: %v", err)
	}
	defer cancel()

	// Then
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected status context to have a deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > time.Second {
		t.Fatalf("expected a one-second deadline, got %s", remaining)
	}
}

func TestContextWithStatusTimeoutRejectsNonPositiveSeconds(t *testing.T) {
	t.Parallel()

	for _, timeoutSeconds := range []int64{0, -1} {
		// When
		ctx, cancel, err := contextWithStatusTimeout(context.Background(), timeoutSeconds)

		// Then
		if err == nil {
			cancel()
			t.Fatalf("expected timeout %d to be rejected", timeoutSeconds)
		}
		if ctx != nil || cancel != nil {
			t.Fatalf("expected no context for timeout %d", timeoutSeconds)
		}
		if !strings.Contains(err.Error(), "--timeout must be positive") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestContextWithStatusTimeoutRejectsOverflow(t *testing.T) {
	t.Parallel()

	// When
	ctx, cancel, err := contextWithStatusTimeout(
		context.Background(),
		maxStatusTimeoutSeconds+1,
	)

	// Then
	if err == nil || ctx != nil || cancel != nil {
		t.Fatalf("expected oversized timeout to be rejected, got context=%v error=%v", ctx, err)
	}
}

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
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	// Then
	if stderr.String() != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Status: initialized\n") {
		t.Fatalf("expected status output on stdout, got %q", stdout.String())
	}
}
