// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
)

func TestRequireDeploymentFileLogging(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "deploy"}
	if deploymentFileLoggingIsRequired(cmd) {
		t.Fatal("expected deployment file logging to be disabled by default")
	}

	requireDeploymentFileLogging(cmd)
	if !deploymentFileLoggingIsRequired(cmd) {
		t.Fatal("expected deployment file logging to be required after annotation")
	}
}

func TestDeploymentLogSessionStartsAfterInit(t *testing.T) {
	t.Parallel()

	if !deploymentLogSessionStartsAfterInit(&cobra.Command{Use: commandInit}) {
		t.Fatal("expected init to defer deployment log setup")
	}
	if !deploymentLogSessionStartsAfterInit(&cobra.Command{Use: commandInstall}) {
		t.Fatal("expected install to defer deployment log setup")
	}
	if deploymentLogSessionStartsAfterInit(&cobra.Command{Use: "deploy"}) {
		t.Fatal("expected deploy to start deployment log setup in root pre-run")
	}
}

func TestStartDeploymentLogSession_CreatesDeploymentLogWithoutState(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()

	// When
	cleanup, err := startDeploymentLogSession(context.Background(), "deploy", deploymentDir)
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}
	t.Cleanup(cleanup)

	logFilePath := deploymentLogFilePath(deploymentDir)

	// Then
	if _, err := os.Stat(logFilePath); err != nil {
		t.Fatalf("expected log file to exist at %q: %v", logFilePath, err)
	}
	if _, err := os.Stat(
		filepath.Join(deploymentDir,
			config.ExasolPersonalStateFileName)); !os.IsNotExist(
		err,
	) {
		t.Fatalf("expected state file to remain untouched, got err=%v", err)
	}

	content, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "deployment log session started") {
		t.Fatalf("expected bootstrap entry in log file, got: %q", string(content))
	}
}

func TestStartDeploymentLogSession_CreatesLogFileWhenDeploymentDirectoryDoesNotExistYet(
	t *testing.T,
) {
	t.Parallel()

	// Given
	parentDir := t.TempDir()
	deploymentDir := filepath.Join(parentDir, "deployment")

	// When
	cleanup, err := startDeploymentLogSession(context.Background(), "init", deploymentDir)
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}
	t.Cleanup(cleanup)

	// Then
	if _, err := os.Stat(deploymentLogFilePath(deploymentDir)); err != nil {
		t.Fatalf("expected log file to be created, got err=%v", err)
	}
}
