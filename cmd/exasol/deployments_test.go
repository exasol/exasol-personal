// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_EmptyWhenRootMissing(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())

	entries, err := listDeploymentDirectories()
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

	entries, err := listDeploymentDirectories()
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

	entries, err := listDeploymentDirectories()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploymentStatusNotInitialized {
		t.Fatalf("expected a single not_initialized entry, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_ReportsInitializedForLegacyMarkerOnlyDirectory(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Chdir(t.TempDir())
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	legacyDir := filepath.Join(deploymentsRoot, "legacy")
	mkdirTest(t, legacyDir)
	writeTestMarker(t, filepath.Join(legacyDir, legacyWorkflowStateMarker))

	entries, err := listDeploymentDirectories()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != deploymentStatusInitialized {
		t.Fatalf("expected a single initialized entry, got: %#v", entries)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_MarksActiveEntryFromCwd(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	activeDir := filepath.Join(deploymentsRoot, "staging")
	mkdirTest(t, activeDir)
	writeTestMarker(t, filepath.Join(activeDir, config.ExasolPersonalStateFileName))
	mkdirTest(t, filepath.Join(deploymentsRoot, "prod-aws"))
	t.Chdir(activeDir)

	entries, err := listDeploymentDirectories()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	activeCount := 0
	for _, entry := range entries {
		if !entry.Active {
			continue
		}

		activeCount++
		if entry.Name != "staging" {
			t.Fatalf("expected staging to be active, got: %q", entry.Name)
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly one active entry, got: %d", activeCount)
	}
}

//nolint:paralleltest // t.Chdir and t.Setenv change process state.
func TestListDeploymentDirectories_NoEntryActiveWhenActiveDirOutsideListedTree(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	deploymentsRoot := filepath.Join(config.LauncherDirPath(home), "deployments")
	mkdirTest(t, filepath.Join(deploymentsRoot, "staging"))
	outsideDir := t.TempDir()
	writeTestMarker(t, filepath.Join(outsideDir, config.ExasolPersonalStateFileName))
	t.Chdir(outsideDir)

	entries, err := listDeploymentDirectories()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	for _, entry := range entries {
		if entry.Active {
			t.Fatalf("expected no entry to be active, got: %#v", entry)
		}
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
