// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/tofu"
)

func isPlaceholderTofuArchiveError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "embedded tofu archive appears to be a placeholder")
}

func assertInitializedState(
	t *testing.T,
	deploymentDir string,
	expectedVersion string,
) {
	t.Helper()

	state, err := config.ReadExasolPersonalState(deploymentDir)
	if err != nil {
		t.Fatalf("expected workflow state to be readable, got error: %v", err)
	}
	if state.DeploymentVersion != expectedVersion {
		t.Fatalf(
			"expected deployment version to be persisted as %q, got %q",
			expectedVersion,
			state.DeploymentVersion,
		)
	}
	if strings.TrimSpace(state.DeploymentId) == "" {
		t.Fatal("expected deploymentId to be persisted, got empty")
	}
	if strings.TrimSpace(state.ClusterIdentity) == "" {
		t.Fatal("expected clusterIdentity to be persisted, got empty")
	}
	if ver, ok, err := config.ReadDeploymentVersionMarker(deploymentDir); err != nil {
		t.Fatalf("expected deployment version marker to be readable, got error: %v", err)
	} else if !ok {
		t.Fatalf("expected deployment version marker %q to exist",
			config.DeploymentVersionMarkerFileName)
	} else if ver != expectedVersion {
		t.Fatalf("expected deployment version marker to be %q, got %q", expectedVersion, ver)
	}
	workflowState, err := state.GetWorkflowState()
	if err != nil {
		t.Fatalf("expected workflow state to be set, got error: %v", err)
	}
	if _, ok := workflowState.(*config.WorkflowStateInitialized); !ok {
		t.Fatalf("expected Initialized workflow state, got %T", workflowState)
	}
}

func TestInitDeployment_CreatesTfVarsWhenTofuConfigured(t *testing.T) {
	t.Parallel()

	// Given a deployment directory
	deploymentDir := t.TempDir()

	// When the deployment is intialized
	err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{"cluster_size": "2"},
		map[string]string{},
		deploymentDir,
		false,
		"0.0.0",
	)
	if err != nil && !isPlaceholderTofuArchiveError(err) {
		t.Fatalf("InitDeployment failed: %v", err)
	}

	// In placeholder mode (common for unit tests), InitDeployment fails at tofu
	// extraction. In that mode, only assertions up to tfvars generation are valid.
	if err == nil {
		assertInitializedState(t, deploymentDir, "0.0.0")
	}

	// Then: tfvars file exists at default path (per manifest)
	tfvarsPath := filepath.Join(
		deploymentDir,
		config.InfrastructureFilesDirectory,
		tofu.DefaultVarsOutput,
	)
	data, err := os.ReadFile(tfvarsPath)
	if err != nil {
		t.Fatalf("expected %s to exist, got read error: %v", tfvarsPath, err)
	}
	content := string(data)

	if !strings.Contains(content, "cluster_size") {
		t.Fatalf("expected tfvars to contain cluster_size, got: %s", content)
	}

	if !strings.Contains(content, "infrastructure_artifact_dir") {
		t.Fatalf("expected tfvars to contain infrastructure_artifact_dir, got: %s", content)
	}

	if !strings.Contains(content, "installation_preset_dir") {
		t.Fatalf("expected tfvars to contain installation_preset_dir, got: %s", content)
	}

	if err != nil {
		return
	}

	// Then: installation variables file exists at the manifest-defined path.
	installVarsPath := filepath.Join(
		deploymentDir,
		config.InstallationFilesDirectory,
		"files/etc/exasol_launcher/installation.json",
	)
	if _, err := os.Stat(installVarsPath); err != nil {
		t.Fatalf("expected installation variables file %s to exist, got: %v", installVarsPath, err)
	}
	installData, err := os.ReadFile(installVarsPath)
	if err != nil {
		t.Fatalf("expected %s to exist, got read error: %v", installVarsPath, err)
	}
	installContent := string(installData)
	if !strings.Contains(installContent, "\"deployment_id\"") {
		t.Fatalf("expected installation vars to contain deployment_id, got: %s", installContent)
	}
	if !strings.Contains(installContent, "\"cluster_identity\"") {
		t.Fatalf("expected installation vars to contain cluster_identity, got: %s", installContent)
	}
	if !strings.Contains(installContent, "\"version_check_url\"") {
		t.Fatalf("expected installation vars to contain version_check_url, got: %s", installContent)
	}
}

func TestInitDeployment_CreatesDeploymentDir(t *testing.T) {
	t.Parallel()

	// Given
	root := t.TempDir()
	deploymentDir := filepath.Join(root, "deployment")

	// When
	err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deploymentDir,
		false,
		"",
	)
	// Then
	if err != nil && !isPlaceholderTofuArchiveError(err) {
		t.Fatalf("InitDeployment failed: %v", err)
	}

	info, _ := os.Stat(deploymentDir)
	if !info.IsDir() {
		t.Fatal("Deployment directory does not exist")
	}
}

func TestInitDeployment_ErrWhenDirNotEmpty(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	nonEmptyPath := filepath.Join(deploymentDir, "not-empty")
	if err := os.WriteFile(nonEmptyPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write preexisting file failed: %v", err)
	}

	// When
	err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deploymentDir,
		false,
		"",
	)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), ErrDeploymentDirectoryNotEmpty.Error()) {
		t.Fatalf("expected ErrDeploymentDirectoryNotEmpty, got: %v", err)
	}
}
