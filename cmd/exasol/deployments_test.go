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
	"github.com/exasol/exasol-personal/internal/deploy"
)

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_EmptyWhenRootMissing(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_SortsAlphabeticallyAndIgnoresNonDirectories(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	mkdirTest(t, filepath.Join(deploymentsRoot, "staging"))
	mkdirTest(t, filepath.Join(deploymentsRoot, "prod-aws"))
	writeTestMarker(t, filepath.Join(deploymentsRoot, "not-a-directory"))

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got: %#v", entries)
	}
	if entries[0].Name != "prod-aws" || entries[1].Name != "staging" {
		t.Fatalf("expected alphabetical order, got: %q, %q", entries[0].Name, entries[1].Name)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsNotInitializedForUnrecognizedDirectory(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	mkdirTest(t, filepath.Join(deploymentsRoot, "empty"))

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploy.StatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsNotInitializedForLegacyMarkerOnlyDirectory(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	legacyDir := filepath.Join(deploymentsRoot, "legacy")
	mkdirTest(t, legacyDir)
	writeTestMarker(t, filepath.Join(legacyDir, legacyWorkflowStateMarker))

	// A directory recognized only via the legacy .workflowState.json marker
	// (no modern state file) has no lifecycle state for deploy.GetStatus to
	// read, so it is reported as not_initialized.
	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploy.StatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsUnparseableStateFileAsNotInitialized(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	corruptDir := filepath.Join(deploymentsRoot, "corrupt")
	mkdirTest(t, corruptDir)
	writeTestMarker(t, filepath.Join(corruptDir, config.ExasolPersonalStateFileName))

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploy.StatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
	if entries[0].Infrastructure != "" || entries[0].Installation != "" {
		t.Fatalf("expected no preset identity, got: %#v", entries[0])
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsRunningStatusAndPresetIdentity(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	runningDir := filepath.Join(deploymentsRoot, "running")
	mkdirTest(t, runningDir)
	writeRunningStateWithPresetIdentity(t, runningDir, "name:aws", "name:standard")

	entries, err := listDeploymentDirectories(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected a single entry, got: %#v", entries)
	}
	if entries[0].Status != deploy.StatusRunning {
		t.Fatalf("expected status %q, got %#v", deploy.StatusRunning, entries[0])
	}
	if entries[0].Infrastructure != "aws" || entries[0].Installation != "standard" {
		t.Fatalf("expected preset identity to be displayed, got: %#v", entries[0])
	}
}

func TestDeploymentListEntryText_IncludesPresetWhenPresent(t *testing.T) {
	t.Parallel()

	entry := deploymentListEntry{
		Name:           "prod",
		Path:           "/deployments/prod",
		Status:         deploy.StatusRunning,
		Infrastructure: "aws",
		Installation:   "standard",
	}

	got := deploymentListEntryText(entry)
	want := "prod status=running preset=aws/standard path=/deployments/prod"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDeploymentListEntryText_OmitsPresetWhenAbsent(t *testing.T) {
	t.Parallel()

	entry := deploymentListEntry{
		Name:   "empty",
		Path:   "/deployments/empty",
		Status: deploy.StatusNotInitialized,
	}

	got := deploymentListEntryText(entry)
	want := "empty status=not_initialized path=/deployments/empty"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRenderDeploymentsListText_ReportsNoDeploymentsWhenEmpty(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	if err := renderDeploymentsListText(&out, nil); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.String() != "No deployment directories found.\n" {
		t.Fatalf("expected the empty-state message, got %q", out.String())
	}
}

func TestRenderDeploymentsListText_RendersOneLinePerEntry(t *testing.T) {
	t.Parallel()

	entries := []deploymentListEntry{
		{Name: "a", Path: "/a", Status: deploy.StatusRunning},
		{Name: "b", Path: "/b", Status: deploy.StatusStopped},
	}

	var out strings.Builder
	if err := renderDeploymentsListText(&out, entries); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	want := "a status=running path=/a\nb status=stopped path=/b\n"
	if out.String() != want {
		t.Fatalf("expected %q, got %q", want, out.String())
	}
}

func TestDeploymentsListCommand_HasNoDeploymentDirOrNameFlag(t *testing.T) {
	t.Parallel()

	if deploymentsListCmd.Flags().Lookup(deploymentDirFlagName) != nil {
		t.Fatal("expected deployments list to not register --deployment-dir")
	}
	if deploymentsListCmd.Flags().Lookup(deploymentNameFlagName) != nil {
		t.Fatal("expected deployments list to not register --deployment")
	}
}

func mkdirTest(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
}

func writeRunningStateWithPresetIdentity(
	t *testing.T,
	deploymentPath string,
	infrastructureIdentity string,
	installationIdentity string,
) {
	t.Helper()

	deployment := config.NewDeploymentDir(deploymentPath)
	state := &config.ExasolPersonalState{
		InfrastructurePresetIdentity: infrastructureIdentity,
		InstallationPresetIdentity:   installationIdentity,
	}
	err := state.SetWorkflowStateAndWrite(&config.WorkflowStateRunning{}, deployment)
	if err != nil {
		t.Fatalf("failed to write running state: %v", err)
	}
}
