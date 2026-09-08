// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/launcherpaths"
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
	deploymentsRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	expected := filepath.Join(deploymentsRoot, "default")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_NamedFlagWins(t *testing.T) {
	// Given: the current working directory is not recognized as a deployment directory.
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)
	cmd, state := commandWithDeploymentSelection(t, "", "staging")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the named deployment directory is used.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceNamed {
		t.Fatalf("expected named source, got %v", source)
	}
	deploymentsRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	expected := filepath.Join(deploymentsRoot, "staging")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

//nolint:paralleltest // t.Chdir changes process state.
func TestResolveDeploymentDir_NamedFlagWinsOverCurrentDeploymentDir(t *testing.T) {
	// Given: the current working directory is itself a different recognized deployment directory.
	home := t.TempDir()
	cwd := t.TempDir()
	setTestHome(t, home)
	t.Chdir(cwd)
	writeTestMarker(t, filepath.Join(cwd, config.ExasolPersonalStateFileName))
	cmd, state := commandWithDeploymentSelection(t, "", "staging")

	// When: deployment directory resolution runs.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the explicit --deployment flag wins over the current directory.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceNamed {
		t.Fatalf("expected named source, got %v", source)
	}
	deploymentsRoot, err := launcherpaths.DeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve deployments root: %v", err)
	}
	expected := filepath.Join(deploymentsRoot, "staging")
	if deployment.Root() != expected {
		t.Fatalf("expected %q, got %q", expected, deployment.Root())
	}
}

//nolint:paralleltest // home-directory environment is process-global.
func TestResolveDeploymentDir_MigratedSourceForMovedLegacyPath(t *testing.T) {
	// Given: a deployment that migration moved from the legacy root to the new root.
	legacyRoot := seedMovedDeployment(t, t.TempDir())

	// When: deployment directory resolution runs with the legacy path.
	// Then: the moved deployment directory is used.
	requireMigratedResolution(t, filepath.Join(legacyRoot, "staging"))
}

//nolint:paralleltest // home-directory environment is process-global.
func TestResolveDeploymentDir_LegacyPathThroughSymlinkFollowsMigratedDeployment(t *testing.T) {
	// Given: a moved deployment and a symbolic link to the home directory.
	home := t.TempDir()
	legacyRoot := seedMovedDeployment(t, home)
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(home, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	relRoot, err := filepath.Rel(home, legacyRoot)
	if err != nil {
		t.Fatalf("failed to relate legacy root to home: %v", err)
	}

	// When: deployment directory resolution runs with the legacy path through the link.
	// Then: the moved deployment directory is used.
	requireMigratedResolution(t, filepath.Join(link, relRoot, "staging"))
}

//nolint:paralleltest // home-directory environment and caseInsensitivePaths are process-global.
func TestResolveDeploymentDir_LegacyPathInOtherCaseFollowsMigratedDeployment(t *testing.T) {
	// Given: a moved deployment on a case-insensitive file system.
	legacyRoot := seedMovedDeployment(t, t.TempDir())
	previous := caseInsensitivePaths
	caseInsensitivePaths = true
	t.Cleanup(func() { caseInsensitivePaths = previous })
	upper := filepath.Join(filepath.Dir(legacyRoot), strings.ToUpper(filepath.Base(legacyRoot)))

	// When: deployment directory resolution runs with the legacy path in upper case.
	// Then: the moved deployment directory is used.
	requireMigratedResolution(t, filepath.Join(upper, "staging"))
}

func seedMovedDeployment(t *testing.T, home string) string {
	t.Helper()
	setTestHome(t, home)
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve legacy deployments root: %v", err)
	}
	moved, err := launcherpaths.NamedDeploymentDirPath("staging")
	if err != nil {
		t.Fatalf("failed to resolve new deployment path: %v", err)
	}
	if err := os.MkdirAll(moved, 0o700); err != nil {
		t.Fatalf("failed to create moved deployment: %v", err)
	}

	return legacyRoot
}

func requireMigratedResolution(t *testing.T, legacyPath string) {
	t.Helper()
	moved, err := launcherpaths.NamedDeploymentDirPath("staging")
	if err != nil {
		t.Fatalf("failed to resolve new deployment path: %v", err)
	}
	cmd, state := commandWithDeploymentSelection(t, legacyPath, "")
	deployment, source, err := resolveDeploymentDir(cmd, state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceMigrated {
		t.Fatalf("expected migrated source, got %v", source)
	}
	if deployment.Root() != moved {
		t.Fatalf("expected %q, got %q", moved, deployment.Root())
	}
}

//nolint:paralleltest // home-directory environment is process-global.
func TestResolveDeploymentDir_ExplicitLegacyPathStaysWhileItExists(t *testing.T) {
	// Given: a deployment name at both the legacy and the new root.
	setTestHome(t, t.TempDir())
	legacyRoot, err := launcherpaths.LegacyDeploymentsRootPath()
	if err != nil {
		t.Fatalf("failed to resolve legacy deployments root: %v", err)
	}
	legacy := filepath.Join(legacyRoot, "staging")
	moved, err := launcherpaths.NamedDeploymentDirPath("staging")
	if err != nil {
		t.Fatalf("failed to resolve new deployment path: %v", err)
	}
	for _, dir := range []string{legacy, moved} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
	}
	cmd, state := commandWithDeploymentSelection(t, legacy, "")

	// When: deployment directory resolution runs with the legacy path.
	deployment, source, err := resolveDeploymentDir(cmd, state)
	// Then: the legacy path is used as given.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != deploymentDirSourceExplicit {
		t.Fatalf("expected explicit source, got %v", source)
	}
	if deployment.Root() != legacy {
		t.Fatalf("expected %q, got %q", legacy, deployment.Root())
	}
}

func TestResolveDeploymentDirFromValues_DeploymentDirWinsOverNameWhenBothChanged(t *testing.T) {
	t.Parallel()

	// Given: both --deployment-dir and --deployment are marked as explicitly set.
	explicit := filepath.Join(t.TempDir(), "explicit")
	values := deploymentDirFlagValues{
		deploymentDir:        explicit,
		deploymentDirChanged: true,
		name:                 "staging",
		nameChanged:          true,
	}

	// When: the shared precedence resolver runs.
	deployment, source, err := resolveDeploymentDirFromValues(values)
	// Then: --deployment-dir wins.
	//
	// Real command execution never reaches this combination: Cobra's
	// MarkFlagsMutuallyExclusive plus the early ValidateFlagGroups call in root's
	// PersistentPreRunE reject it first. This deterministic precedence only
	// matters for the harmless pre-Cobra raw-args pre-scan.
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

func TestRegisterDeploymentDirFlag_DeploymentDirAndNameAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	// Given: a command with both flags set.
	cmd, _ := commandWithDeploymentSelection(t, "/tmp/explicit", "staging")

	// When: flag groups are validated.
	err := cmd.ValidateFlagGroups()
	// Then: it fails, naming both flags.
	if err == nil {
		t.Fatal("expected an error for setting both --deployment-dir and --deployment")
	}
	if !strings.Contains(err.Error(), deploymentDirFlagName) ||
		!strings.Contains(err.Error(), deploymentNameFlagName) {
		t.Fatalf("expected error to mention both flags, got: %v", err)
	}
}

func commandWithDeploymentSelection(
	t *testing.T,
	deploymentDirValue string,
	nameValue string,
) (*cobra.Command, *CommonFlags) {
	t.Helper()

	state := &CommonFlags{}
	cmd := &cobra.Command{Use: "test"}
	registerDeploymentDirFlag(cmd, state)
	if deploymentDirValue != "" {
		if err := cmd.Flags().Set(deploymentDirFlagName, deploymentDirValue); err != nil {
			t.Fatalf("failed to set deployment-dir flag: %v", err)
		}
	}
	if nameValue != "" {
		if err := cmd.Flags().Set(deploymentNameFlagName, nameValue); err != nil {
			t.Fatalf("failed to set name flag: %v", err)
		}
	}

	return cmd, state
}

func setTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
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
