// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localports"
)

func TestAppendDeployFailureHint_AddsLauncherLogPath(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("deployment failed")
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	err := appendDeployFailureHint(deployment, baseErr)

	// Then
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("expected wrapped error to match base error, got: %v", err)
	}
	if !strings.Contains(err.Error(), deployment.Resolve("deployment.log")) {
		t.Fatalf("expected error to include deployment log path, got: %q", err.Error())
	}
}

func TestAppendDeployFailureHint_AddsDeploymentInfoPathWhenPresent(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("deployment failed")
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Connection: &config.DeploymentConnection{
			Host:           "example.local",
			DisplayHost:    "example.local",
			DBPort:         8563,
			UIPort:         8443,
			Username:       "sys",
			ShellSupported: true,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	// When
	err := appendDeployFailureHint(deployment, baseErr)

	// Then
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), deployment.NodeDetailsPath()) {
		t.Fatalf("expected error to include deployment info path, got: %q", err.Error())
	}
}

func TestAppendDeployFailureHintNilInput(t *testing.T) {
	t.Parallel()

	if err := appendDeployFailureHint(config.NewDeploymentDir(t.TempDir()), nil); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

//nolint:paralleltest // This test temporarily replaces the process-wide default logger.
func TestRecordDeployFailure_LogsFailureAndRuntimeOutput(t *testing.T) {
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	stderr := "internal:0:0-0: Error: Could not process rule: No such file or directory\n" +
		"Error: netavark: nftables error"
	runtimeErr := fmt.Errorf("failed to start Nano container: %w", &localinstall.CommandError{
		Stderr: stderr, Err: errors.New("exit status 126"),
	})
	logCapture := &logCaptureHandler{}
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(logCapture))
	defer slog.SetDefault(originalLogger)

	// When
	_ = recordDeployFailure(&config.ExasolPersonalState{}, deployment, runtimeErr)

	// Then
	logged := loggedAttrs(logCapture)
	if logged["deployment failed/error"] != runtimeErr.Error() {
		t.Fatalf("expected the failure to be logged, got %#v", logged)
	}
	if logged["container runtime command output/stderr"] != stderr {
		t.Fatalf("expected the runtime output to be logged, got %#v", logged)
	}
}

//nolint:paralleltest // This test temporarily replaces the process-wide default logger.
func TestRunDeployBackend_LogsRuntimeOutputForUnavailablePort(t *testing.T) {
	// Given
	deployment, state := deploymentInState(
		t,
		&config.WorkflowStateOperationInProgress{Operation: config.DeployOperation},
	)
	if err := os.MkdirAll(deployment.InstallationDir(), 0o700); err != nil {
		t.Fatalf("create installation dir failed: %v", err)
	}
	writeTestFile(t, deployment.InstallManifestPath(), `
name: Test Installation
description: test installation
install: []
`)
	stderr := "Error: rootlessport listen tcp 127.0.0.1:28563: bind: address already in use"
	backend := unavailablePortBackend{err: &localports.UnavailableError{
		Service: "db",
		Port:    28563,
		Cause: &localinstall.CommandError{
			Stderr: stderr, Err: errors.New("exit status 126"),
		},
	}}
	logCapture := &logCaptureHandler{}
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(logCapture))
	defer slog.SetDefault(originalLogger)

	// When
	_ = runDeployBackend(
		context.Background(), state, deployment, backend, io.Discard, DeployOptions{},
	)

	// Then
	logged := loggedAttrs(logCapture)
	if logged["container runtime command output/stderr"] != stderr {
		t.Fatalf("expected the runtime output to be logged, got %#v", logged)
	}
}

func loggedAttrs(capture *logCaptureHandler) map[string]string {
	logged := map[string]string{}
	for _, record := range capture.records {
		record.Attrs(func(attr slog.Attr) bool {
			logged[record.Message+"/"+attr.Key] = attr.Value.String()
			return true
		})
	}

	return logged
}
