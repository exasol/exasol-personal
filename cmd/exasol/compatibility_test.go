// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEnforceDeploymentDirectoryCompatibility_FailsEarlyWhenNotInitialized(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	cmd := &cobra.Command{Use: "deploy"}
	requireDeploymentCompatibility(cmd, minSupportedDeploymentVersionBaseline)
	requireInitializedDeploymentDir(cmd)

	err := enforceDeploymentDirectoryCompatibility(cmd, tmp)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "deployment directory is not initialized") {
		t.Fatalf("unexpected error message: %q", msg)
	}
	if !strings.Contains(msg, ".exasolLauncherState.json") {
		t.Fatalf("expected error to mention state file, got: %q", msg)
	}
	if !strings.Contains(msg, "exasol init") || !strings.Contains(msg, "exasol install") {
		t.Fatalf("expected error to mention init/install guidance, got: %q", msg)
	}
}

func TestEnforceDeploymentDirectoryCompatibility_HintsLegacyWorkflowStateLayout(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Simulate legacy deployment directories created before state file
	// .exasolLauncherState.json existed.
	err := os.WriteFile(filepath.Join(tmp, ".workflowState.json"), []byte("{}"), 0o600)
	if err != nil {
		t.Fatalf("failed to create legacy workflow state file: %v", err)
	}

	cmd := &cobra.Command{Use: "some_init_like_command"}
	requireDeploymentCompatibility(cmd, minSupportedDeploymentVersionBaseline)
	// Note: init/install must NOT require an initialized deployment dir.
	err = enforceDeploymentDirectoryCompatibility(cmd, tmp)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "deployment directory appears to be from an older version") {
		t.Fatalf("expected legacy version hint, got: %q", msg)
	}
	if !strings.Contains(msg, ".workflowState.json") {
		t.Fatalf("expected message to mention legacy file, got: %q", msg)
	}
	if !strings.Contains(msg, "1.0.0") {
		t.Fatalf("expected message to suggest older launcher version, got: %q", msg)
	}
}
