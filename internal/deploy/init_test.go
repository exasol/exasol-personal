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
		deploymentDir,
		false,
		"0.0.0",
	)
	if err != nil {
		t.Fatalf("InitDeployment failed: %v", err)
	}

	// Then: workflow state exists and is readable
	state, err := config.ReadExasolPersonalState(deploymentDir)
	if err != nil {
		t.Fatalf("expected workflow state to be readable, got error: %v", err)
	}
	workflowState, err := state.GetWorkflowState()
	if err != nil {
		t.Fatalf("expected workflow state to be set, got error: %v", err)
	}
	if _, ok := workflowState.(*config.WorkflowStateInitialized); !ok {
		t.Fatalf("expected Initialized workflow state, got %T", workflowState)
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
