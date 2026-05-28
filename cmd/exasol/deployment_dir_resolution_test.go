// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
)

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_ExplicitFlagWins(t *testing.T) {
	// Given: a current deployment directory and an explicit deployment directory flag.
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeTestMarker(t, filepath.Join(cwd, config.ExasolPersonalStateFileName))

	explicit := filepath.Join(t.TempDir(), "explicit")
	cmd, state := commandWithDeploymentDirFlag(t, explicit)

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the explicit flag value is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceExplicit {
		t.Fatalf("expected explicit source, got %v", source)
	}
	if deployment.Root() != explicit {
		t.Fatalf("expected %q, got %q", explicit, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_CurrentDeploymentDirWinsWhenFlagOmitted(t *testing.T) {
	// Given: the current working directory is recognized as a deployment directory.
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeTestMarker(t, filepath.Join(cwd, config.DeploymentVersionMarkerFileName))
	cmd, state := commandWithDeploymentDirFlag(t, "")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the current working directory is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceCurrent {
		t.Fatalf("expected current directory source, got %v", source)
	}
	if deployment.Root() != cwd {
		t.Fatalf("expected %q, got %q", cwd, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_DefaultWinsWhenFlagOmittedOutsideDeploymentDir(t *testing.T) {
	// Given: the current working directory is not recognized as a deployment directory.
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)
	cmd, state := commandWithDeploymentDirFlag(t, "")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the default deployment directory is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceDefault {
		t.Fatalf("expected default source, got %v", source)
	}
	expected := filepath.Join(home, ".exasol", "personal", "deployments", "default")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

func setTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}

func TestResolveDeploymentDir_IgnoresCommandsWithoutDeploymentDirFlag(t *testing.T) {
	t.Parallel()

	// Given: a command that does not operate on deployment directories.
	cmd := &cobra.Command{Use: "version"}
	state := &CommonFlags{}

	// When: deployment directory resolution runs.
	_, source, err := resolveDeploymentDir(cmd, state)
	// Then: no deployment directory source is selected.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceNone {
		t.Fatalf("expected no source, got %v", source)
	}
}

func commandWithDeploymentDirFlag(t *testing.T, value string) (*cobra.Command, *CommonFlags) {
	t.Helper()

	state := &CommonFlags{}
	cmd := &cobra.Command{Use: "test"}
	registerDeploymentDirFlag(cmd, state)
	if value != "" {
		if err := cmd.Flags().Set(deploymentDirFlagName, value); err != nil {
			t.Fatalf("failed to set deployment-dir flag: %v", err)
		}
	}

	return cmd, state
}

func writeTestMarker(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}
}
