// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/tofu"
)

func TestEnsureDeploymentPresetIdentityMatches_RejectsDifferentPreset(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("initial init failed: %v", err)
	}

	// When
	err := EnsureDeploymentPresetIdentityMatches(
		deployment,
		PresetRef{Name: "stackit"},
		PresetRef{Name: presets.DefaultInstallation},
	)

	// Then
	if !errors.Is(err, ErrDeploymentPresetMismatch) {
		t.Fatalf("expected preset mismatch, got %v", err)
	}
}

func TestSetDeploymentConfiguration_UpdatesVariablesAndPreservesStateFiles(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{"cluster_size": "2"},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	statePath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultStateFile)
	if err := os.WriteFile(statePath, []byte("state"), 0o600); err != nil {
		t.Fatalf("write state file failed: %v", err)
	}
	mgr, err := runtimeartifacts.NewResourceManagerWithSpec(resources.ResourcesYAML)
	if err != nil {
		t.Fatalf("create artifact manager: %v", err)
	}
	tofuBinaryPath, err := mgr.Request(context.Background(), "tofu")
	if err != nil {
		t.Fatalf("resolve tofu binary path failed: %v", err)
	}
	if err := os.Remove(tofuBinaryPath); err != nil {
		t.Fatalf("remove tofu binary before config set failed: %v", err)
	}

	// When
	if _, err := SetDeploymentConfiguration(
		context.Background(),
		map[string]string{"cluster_size": "3"},
		map[string]string{},
		deployment,
	); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	// Then
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to be preserved, got %v", err)
	}
	if _, err := os.Stat(tofuBinaryPath); !os.IsNotExist(err) {
		t.Fatalf("expected config set not to recreate tofu binary, got %v", err)
	}
	tfvarsPath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultVarsOutput)
	content, err := os.ReadFile(tfvarsPath)
	if err != nil {
		t.Fatalf("read tfvars failed: %v", err)
	}
	if !strings.Contains(string(content), `cluster_size`) ||
		!strings.Contains(string(content), `= 3`) {
		t.Fatalf("expected updated cluster size, got: %s", string(content))
	}
}

func TestSetDeploymentConfiguration_PreservesDeploymentCreatedAt(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{"cluster_size": "2"},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	tfvarsPath := filepath.Join(deployment.InfrastructureDir(), tofu.DefaultVarsOutput)
	createdAt := "2001-02-03T04:05:06Z"
	parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		t.Fatalf("parse createdAt failed: %v", err)
	}
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	state.CreatedAt = parsedCreatedAt
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("write state failed: %v", err)
	}

	// When
	if _, err := SetDeploymentConfiguration(
		context.Background(),
		map[string]string{"cluster_size": "3"},
		map[string]string{},
		deployment,
	); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	// Then
	content, err := os.ReadFile(tfvarsPath)
	if err != nil {
		t.Fatalf("read tfvars failed: %v", err)
	}
	if !deploymentCreatedAtPattern(createdAt).Match(content) {
		t.Fatalf("expected deployment_created_at to be preserved, got:\n%s", string(content))
	}
}

func TestWorkflowStatePermitsConfigure_RejectsAllNonInitializedStates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		state any
	}{
		{name: "running", state: &config.WorkflowStateRunning{}},
		{name: "stopped", state: &config.WorkflowStateStopped{}},
		{name: "deployment_failed", state: &config.WorkflowStateDeploymentFailed{Error: "boom"}},
		{
			name: "interrupted_during_deploy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DeployOperation,
			},
		},
		{
			name: "interrupted_during_destroy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DestroyOperation,
			},
		},
		{
			name: "operation_in_progress",
			state: &config.WorkflowStateOperationInProgress{
				Operation: config.DeployOperation,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exasolState := &config.ExasolPersonalState{}
			if err := exasolState.SetWorkflowState(test.state); err != nil {
				t.Fatalf("set workflow state failed: %v", err)
			}

			err := WorkflowStatePermitsConfigure(exasolState)

			if !errors.Is(err, ErrConfigureNotAllowed) {
				t.Fatalf("expected ErrConfigureNotAllowed, got %v", err)
			}
			if !strings.Contains(err.Error(), "run `exasol destroy`") {
				t.Fatalf("expected destroy guidance, got %v", err)
			}
			if !strings.Contains(err.Error(), "exasol remove") {
				t.Fatalf("expected remove guidance, got %v", err)
			}
		})
	}
}

func TestWorkflowStatePermitsDeploy_BlockedStatesSurfaceRecoveryGuidance(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		state        any
		sentinel     error
		wantStatus   string
		wantGuidance []string
	}{
		{
			name: "interrupted_during_destroy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DestroyOperation,
			},
			sentinel:     ErrUnexpectedDeploymentStatus,
			wantStatus:   StatusInterrupted,
			wantGuidance: []string{"Interrupted during", "destroy"},
		},
		{
			name:         "stopped",
			state:        &config.WorkflowStateStopped{},
			sentinel:     ErrUnexpectedDeploymentStatus,
			wantStatus:   StatusStopped,
			wantGuidance: []string{"start", "destroy"},
		},
		{
			name: "operation_in_progress_non_deploy",
			state: &config.WorkflowStateOperationInProgress{
				Operation: config.DestroyOperation,
			},
			sentinel:     ErrUnspportedOperation,
			wantStatus:   StatusOperationInProgress,
			wantGuidance: []string{"destroy"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			deployment := config.NewDeploymentDir(t.TempDir())
			writeMinimalInitializedDeployment(t, deployment)
			state, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				t.Fatalf("read state failed: %v", err)
			}
			if err := state.SetWorkflowStateAndWrite(test.state, deployment); err != nil {
				t.Fatalf("write blocked state failed: %v", err)
			}

			// When
			err = WorkflowStatePermitsDeploy(state, deployment)

			// Then
			if err == nil {
				t.Fatal("expected deploy to be blocked, got nil")
			}
			if !errors.Is(err, test.sentinel) {
				t.Fatalf("expected sentinel %v, got %v", test.sentinel, err)
			}
			msg := err.Error()
			if !strings.Contains(msg, test.wantStatus) {
				t.Fatalf("expected message to name state %q, got %q", test.wantStatus, msg)
			}
			if !strings.Contains(msg, deployment.Root()) {
				t.Fatalf(
					"expected message to name deployment dir %q, got %q",
					deployment.Root(),
					msg,
				)
			}
			for _, guidance := range test.wantGuidance {
				if !strings.Contains(msg, guidance) {
					t.Fatalf("expected recovery guidance %q, got %q", guidance, msg)
				}
			}
			if strings.Contains(msg, "unexpected") {
				t.Fatalf("recoverable state must not be labelled \"unexpected\", got %q", msg)
			}
		})
	}
}

func TestWorkflowStatePermitsDeploy_PermittedStates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		state any
	}{
		{name: "initialized", state: &config.WorkflowStateInitialized{}},
		{name: "deployment_failed", state: &config.WorkflowStateDeploymentFailed{Error: "boom"}},
		{name: "running", state: &config.WorkflowStateRunning{}},
		{
			name:  "operation_in_progress_deploy",
			state: &config.WorkflowStateOperationInProgress{Operation: config.DeployOperation},
		},
		{
			name: "interrupted_during_deploy",
			state: &config.WorkflowStateInterrupted{
				InterruptedDuringOperation: config.DeployOperation,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			deployment := config.NewDeploymentDir(t.TempDir())
			writeMinimalInitializedDeployment(t, deployment)
			state, err := config.ReadExasolPersonalState(deployment)
			if err != nil {
				t.Fatalf("read state failed: %v", err)
			}
			if err := state.SetWorkflowStateAndWrite(test.state, deployment); err != nil {
				t.Fatalf("write state failed: %v", err)
			}

			if err := WorkflowStatePermitsDeploy(state, deployment); err != nil {
				t.Fatalf("expected deploy to be permitted, got %v", err)
			}
		})
	}
}

func TestDestroyThenRemoveLocalDeploymentDirectoryRemovesLocalFiles(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)

	// When
	if err := Destroy(context.Background(), deployment, false); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	if err := RemoveLocalDeploymentDirectory(context.Background(), deployment); err != nil {
		t.Fatalf("remove local deployment directory failed: %v", err)
	}

	// Then
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected deployment directory to be removed, got %v", err)
	}
}

func TestDestroyPreservesLocalFilesWhenDestroyFails(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: unknown
`)
	state := newInitializedState(
		false,
		"0.0.0",
		"test-deployment",
		"test-cluster",
		time.Now().UTC(),
		PresetRef{Name: "test-infra"},
		PresetRef{Name: "test-install"},
	)
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatalf("write state failed: %v", err)
	}
	localPath := deployment.Resolve("local.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local file failed: %v", err)
	}

	// When
	destroyErr := Destroy(context.Background(), deployment, false)

	// Then
	if destroyErr == nil {
		t.Fatal("expected destroy to fail, got nil")
	}
	if _, statErr := os.Stat(localPath); statErr != nil {
		t.Fatalf("expected local file to be preserved, got %v", statErr)
	}
	updatedState, readErr := config.ReadExasolPersonalState(deployment)
	if readErr != nil {
		t.Fatalf("read state failed: %v", readErr)
	}
	workflowState, stateErr := updatedState.GetWorkflowState()
	if stateErr != nil {
		t.Fatalf("read workflow state failed: %v", stateErr)
	}
	interrupted, ok := workflowState.(*config.WorkflowStateInterrupted)
	if !ok {
		t.Fatalf("expected interrupted workflow state, got %T", workflowState)
	}
	if interrupted.InterruptedDuringOperation != config.DestroyOperation {
		t.Fatalf("expected destroy interruption, got %q", interrupted.InterruptedDuringOperation)
	}
}

func TestRemoveLocalDeploymentDirectory_RemovesDeploymentDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)
	localPath := deployment.Resolve("local.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local file failed: %v", err)
	}

	// When
	if err := RemoveLocalDeploymentDirectory(context.Background(), deployment); err != nil {
		t.Fatalf("remove local deployment directory failed: %v", err)
	}

	// Then
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected deployment directory to be removed, got %v", err)
	}
}

func TestRemoveLocalDeploymentDirectory_RejectsNonDeploymentDirectory(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localPath := deployment.Resolve("local.txt")
	if err := os.WriteFile(localPath, []byte("local"), 0o600); err != nil {
		t.Fatalf("write local file failed: %v", err)
	}

	// When
	err := RemoveLocalDeploymentDirectory(context.Background(), deployment)

	// Then
	if !errors.Is(err, ErrNotExasolPersonalDeploymentDirectory) {
		t.Fatalf("expected ErrNotExasolPersonalDeploymentDirectory, got %v", err)
	}
	if _, statErr := os.Stat(localPath); statErr != nil {
		t.Fatalf("expected local file to be preserved, got %v", statErr)
	}
}

//nolint:paralleltest // This test intentionally changes process cwd.
func TestRemoveLocalDeploymentDirectory_RejectsCurrentDirectoryInsideDeployment(
	t *testing.T,
) {
	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeMinimalInitializedDeployment(t, deployment)
	cwd := deployment.Resolve("subdir")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("create cwd failed: %v", err)
	}
	t.Chdir(cwd)

	// When
	err := RemoveLocalDeploymentDirectory(context.Background(), deployment)

	// Then
	if !errors.Is(err, ErrDeploymentDirectoryRemovalUnsafe) {
		t.Fatalf("expected ErrDeploymentDirectoryRemovalUnsafe, got %v", err)
	}
	if !strings.Contains(err.Error(), "change to another directory") {
		t.Fatalf("expected cwd guidance, got %v", err)
	}
	if _, statErr := os.Stat(deployment.Root()); statErr != nil {
		t.Fatalf("expected deployment directory to be preserved, got %v", statErr)
	}
}

func TestValidateDeploymentDirectoryRemovalContext_RejectsLauncherBinaryInsideDeployment(
	t *testing.T,
) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	cwd := t.TempDir()
	executable := deployment.Resolve("bin/exasol")

	// When
	err := validateDeploymentDirectoryRemovalContext(deployment, cwd, executable)

	// Then
	if !errors.Is(err, ErrDeploymentDirectoryRemovalUnsafe) {
		t.Fatalf("expected ErrDeploymentDirectoryRemovalUnsafe, got %v", err)
	}
	if !strings.Contains(err.Error(), "move the launcher binary") {
		t.Fatalf("expected launcher binary guidance, got %v", err)
	}
}

func TestValidateDeploymentDirectoryRemovalContext_AllowsOutsidePaths(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	cwd := t.TempDir()
	executable := filepath.Join(t.TempDir(), "exasol")

	// When
	err := validateDeploymentDirectoryRemovalContext(deployment, cwd, executable)
	// Then
	if err != nil {
		t.Fatalf("expected outside paths to be allowed, got %v", err)
	}
}

func TestRemoveLocalDeploymentDirectory_AllowsExtractedPresetManifestsWithoutState(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	if err := os.MkdirAll(deployment.InstallationDir(), 0o700); err != nil {
		t.Fatalf("create installation dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)
	writeTestFile(t, deployment.InstallManifestPath(), `
name: Test Installation
description: test installation
install: []
`)

	// When
	if err := RemoveLocalDeploymentDirectory(context.Background(), deployment); err != nil {
		t.Fatalf("remove local deployment directory failed: %v", err)
	}

	// Then
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected deployment directory to be removed, got %v", err)
	}
}

func writeMinimalInitializedDeployment(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)
	state := newInitializedState(
		false,
		"0.0.0",
		"test-deployment",
		"test-cluster",
		time.Now().UTC(),
		PresetRef{Name: "test-infra"},
		PresetRef{Name: "test-install"},
	)
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatalf("write state failed: %v", err)
	}
}

func deploymentCreatedAtPattern(createdAt string) *regexp.Regexp {
	return regexp.MustCompile(`deployment_created_at\s*=\s*"` + regexp.QuoteMeta(createdAt) + `"`)
}
