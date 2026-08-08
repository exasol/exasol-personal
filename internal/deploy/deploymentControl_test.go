// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/directorymutex"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestWorkflowStatePermitsStart_RequiresStartForStoppedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is stopped.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateStopped{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller should start the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !decision.shouldRun {
		t.Fatal("expected stopped deployment to require backend start")
	}
}

//nolint:paralleltest // Mutates package-level getStatusForStart.
func TestWorkflowStatePermitsStart_SkipsStartForReadyDeployment(t *testing.T) {
	// Given: a running deployment whose database is already ready.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateRunning{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	originalGetStatusForStart := getStatusForStart
	getStatusForStart = func(
		_ context.Context,
		_ config.DeploymentDir,
		checkConnection bool,
	) (*StatusOutput, error) {
		if !checkConnection {
			t.Fatal("expected start check to verify database readiness")
		}

		return &StatusOutput{Status: StatusDatabaseReady}, nil
	}
	t.Cleanup(func() {
		getStatusForStart = originalGetStatusForStart
	})

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller can return successfully without starting the backend again.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected ready deployment to skip backend start")
	}
	if !decision.showConnectionInstructions {
		t.Fatal("expected ready deployment to show connection instructions")
	}
	if !strings.Contains(decision.guidance, "already ready") {
		t.Fatalf("expected already-ready guidance, got %q", decision.guidance)
	}
}

//nolint:paralleltest // Mutates package-level getStatusForStart.
func TestWorkflowStatePermitsStart_GuidesForRunningDeploymentThatIsNotReady(t *testing.T) {
	// Given: a running deployment whose database is not ready.
	deployment := config.NewDeploymentDir(t.TempDir())
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{},
		deployment,
	); err != nil {
		t.Fatalf("write workflow state failed: %v", err)
	}

	originalGetStatusForStart := getStatusForStart
	getStatusForStart = func(
		_ context.Context,
		_ config.DeploymentDir,
		_ bool,
	) (*StatusOutput, error) {
		return &StatusOutput{Status: StatusDatabaseConnectionFailed}, nil
	}
	t.Cleanup(func() {
		getStatusForStart = originalGetStatusForStart
	})

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		deployment,
	)
	// Then: the command gives next-step guidance without retrying the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected non-ready running deployment to skip backend start")
	}
	for _, expected := range []string{"not ready", "exasol status", "exasol stop", "exasol start"} {
		if !strings.Contains(decision.guidance, expected) {
			t.Fatalf("expected guidance to contain %q, got %q", expected, decision.guidance)
		}
	}
}

func TestWorkflowStatePermitsStart_GuidesForInitializedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that has not been deployed yet.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateInitialized{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller gets guidance instead of an unexpected-status error.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected initialized deployment to skip backend start")
	}
	for _, expected := range []string{"initialized", "exasol deploy"} {
		if !strings.Contains(decision.guidance, expected) {
			t.Fatalf("expected guidance to contain %q, got %q", expected, decision.guidance)
		}
	}
}

func TestWorkflowStatePermitsStop_RequiresStopForRunningDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is running.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateRunning{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller should stop the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !decision.shouldRun {
		t.Fatal("expected running deployment to require backend stop")
	}
}

func TestWorkflowStatePermitsStop_GuidesForAlreadyStoppedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is already stopped.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateStopped{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller gets an idempotent no-op decision.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected stopped deployment to skip backend stop")
	}
	if !strings.Contains(decision.guidance, "already stopped") {
		t.Fatalf("expected already-stopped guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStart_ReturnsErrorWhenWorkflowStateUnreadable(t *testing.T) {
	t.Parallel()

	// Given: launcher state with no workflow state set at all.
	exasolState := &config.ExasolPersonalState{}

	// When: start permission is checked.
	_, err := workflowStatePermitsStart(
		context.Background(),
		exasolState,
		config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the unreadable workflow state surfaces as an error.
	if err == nil {
		t.Fatal("expected an error when no workflow state is set")
	}
}

func TestWorkflowStatePermitsStart_GuidesForDeploymentFailed(t *testing.T) {
	t.Parallel()

	// Given: a deployment whose workflow state is failed.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateDeploymentFailed{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(), exasolState, config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller gets guidance instead of retrying the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected a failed deployment to skip backend start")
	}
	if !strings.Contains(decision.guidance, "failed state") {
		t.Fatalf("expected failed-state guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStart_ResumesForOperationInProgressStart(t *testing.T) {
	t.Parallel()

	// Given: an operation already in progress is a start.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateOperationInProgress{
		Operation: config.StartOperation,
	}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(), exasolState, config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller resumes the in-progress start.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !decision.shouldRun {
		t.Fatal("expected an in-progress start to resume")
	}
}

func TestWorkflowStatePermitsStart_SkipsForOperationInProgressOtherOperation(t *testing.T) {
	t.Parallel()

	// Given: an operation already in progress is not a start.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateOperationInProgress{
		Operation: config.DestroyOperation,
	}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(), exasolState, config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller skips start and gets recovery guidance.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected a different in-progress operation to skip start")
	}
	if !strings.Contains(decision.guidance, "exasol status") {
		t.Fatalf("expected recovery guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStart_ResumesForInterruptedStartOrStop(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{config.StartOperation, config.StopOperation} {
		// Given: a workflow interrupted during a start or stop.
		exasolState := &config.ExasolPersonalState{}
		if err := exasolState.SetWorkflowState(&config.WorkflowStateInterrupted{
			InterruptedDuringOperation: operation,
		}); err != nil {
			t.Fatalf("set workflow state failed: %v", err)
		}

		// When: start permission is checked.
		decision, err := workflowStatePermitsStart(
			context.Background(), exasolState, config.NewDeploymentDir(t.TempDir()),
		)
		// Then: the caller resumes via start.
		if err != nil {
			t.Fatalf("expected no error for interrupted %q, got: %v", operation, err)
		}
		if !decision.shouldRun {
			t.Fatalf("expected an interrupted %q to resume via start", operation)
		}
	}
}

func TestWorkflowStatePermitsStart_SkipsForInterruptedOtherOperation(t *testing.T) {
	t.Parallel()

	// Given: a workflow interrupted during an unrelated operation (destroy).
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateInterrupted{
		InterruptedDuringOperation: config.DestroyOperation,
	}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: start permission is checked.
	decision, err := workflowStatePermitsStart(
		context.Background(), exasolState, config.NewDeploymentDir(t.TempDir()),
	)
	// Then: the caller skips start and gets recovery guidance.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected an interrupted destroy to skip start")
	}
	if !strings.Contains(decision.guidance, "exasol status") {
		t.Fatalf("expected recovery guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStop_ReturnsErrorWhenWorkflowStateUnreadable(t *testing.T) {
	t.Parallel()

	// Given: launcher state with no workflow state set at all.
	exasolState := &config.ExasolPersonalState{}

	// Then: stop permission is checked and the unreadable workflow state surfaces as an error.
	if _, err := workflowStatePermitsStop(exasolState); err == nil {
		t.Fatal("expected an error when no workflow state is set")
	}
}

func TestWorkflowStatePermitsStop_GuidesForDeploymentFailed(t *testing.T) {
	t.Parallel()

	// Given: a deployment whose workflow state is failed.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateDeploymentFailed{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller gets guidance instead of retrying the backend.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected a failed deployment to skip backend stop")
	}
	if !strings.Contains(decision.guidance, "failed state") {
		t.Fatalf("expected failed-state guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStop_ResumesForOperationInProgressStop(t *testing.T) {
	t.Parallel()

	// Given: an operation already in progress is a stop.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateOperationInProgress{
		Operation: config.StopOperation,
	}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller resumes the in-progress stop.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !decision.shouldRun {
		t.Fatal("expected an in-progress stop to resume")
	}
}

func TestWorkflowStatePermitsStop_SkipsForOperationInProgressOtherOperation(t *testing.T) {
	t.Parallel()

	// Given: an operation already in progress is not a stop.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateOperationInProgress{
		Operation: config.DestroyOperation,
	}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller skips stop and gets recovery guidance.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected a different in-progress operation to skip stop")
	}
	if !strings.Contains(decision.guidance, "exasol status") {
		t.Fatalf("expected recovery guidance, got %q", decision.guidance)
	}
}

func TestWorkflowStatePermitsStop_ResumesForInterruptedStartStopOrDestroy(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{
		config.StartOperation, config.StopOperation, config.DestroyOperation,
	} {
		// Given: a workflow interrupted during a start, stop, or destroy.
		exasolState := &config.ExasolPersonalState{}
		if err := exasolState.SetWorkflowState(&config.WorkflowStateInterrupted{
			InterruptedDuringOperation: operation,
		}); err != nil {
			t.Fatalf("set workflow state failed: %v", err)
		}

		// When: stop permission is checked.
		decision, err := workflowStatePermitsStop(exasolState)
		// Then: the caller resumes via stop.
		if err != nil {
			t.Fatalf("expected no error for interrupted %q, got: %v", operation, err)
		}
		if !decision.shouldRun {
			t.Fatalf("expected an interrupted %q to resume via stop", operation)
		}
	}
}

func TestWorkflowStatePermitsStop_SkipsForInterruptedOtherOperation(t *testing.T) {
	t.Parallel()

	// Given: a workflow interrupted during an unrelated operation (install).
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateInterrupted{
		InterruptedDuringOperation: "install",
	}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller skips stop and gets recovery guidance.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected an interrupted install to skip stop")
	}
	if !strings.Contains(decision.guidance, "exasol status") {
		t.Fatalf("expected recovery guidance, got %q", decision.guidance)
	}
}

func TestStart_InitializedDeploymentReturnsErrLifecycleActionSkipped(t *testing.T) {
	t.Parallel()

	// Given: a deployment that has been initialized but not deployed.
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
		t.Fatalf("init failed: %v", err)
	}

	// When: Start is called.
	err := Start(context.Background(), deployment, false, 0)
	// Then: the lifecycle action is skipped.
	if !errors.Is(err, ErrLifecycleActionSkipped) {
		t.Fatalf("expected ErrLifecycleActionSkipped, got %v", err)
	}
}

func TestStart_MissingStateReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a deployment with no persisted launcher state.
	deployment := config.NewDeploymentDir(t.TempDir())

	// When: Start is called.
	err := Start(context.Background(), deployment, false, 0)
	// Then: a real error is returned, not a lifecycle skip.
	if err == nil {
		t.Fatal("expected an error when no launcher state has been persisted")
	}
	if errors.Is(err, ErrLifecycleActionSkipped) {
		t.Fatal("expected a real error, not a lifecycle skip")
	}
}

func TestStart_LockedDeploymentReturnsErrLifecycleActionSkipped(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is exclusively locked by another process.
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		t.Fatalf("expected mutex creation to succeed, got: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("expected exclusive lock to succeed, got: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.ReleaseExclusive(context.Background())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// When: Start is called.
	err = Start(ctx, deployment, false, 0)
	// Then: the lifecycle action is skipped.
	if !errors.Is(err, ErrLifecycleActionSkipped) {
		t.Fatalf("expected ErrLifecycleActionSkipped, got %v", err)
	}
}

func TestStop_InitializedDeploymentReturnsNil(t *testing.T) {
	t.Parallel()

	// Given: a deployment that has been initialized but not deployed.
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
		t.Fatalf("init failed: %v", err)
	}

	// Then: calling Stop no-ops without error.
	if err := Stop(context.Background(), deployment, false); err != nil {
		t.Fatalf("expected an initialized-but-not-deployed deployment to no-op, got %v", err)
	}
}

func TestStop_MissingStateReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a deployment with no persisted launcher state.
	deployment := config.NewDeploymentDir(t.TempDir())

	// Then: calling Stop returns an error.
	if err := Stop(context.Background(), deployment, false); err == nil {
		t.Fatal("expected an error when no launcher state has been persisted")
	}
}

func TestStop_LockedDeploymentReturnsNil(t *testing.T) {
	t.Parallel()

	// Given: a deployment that is exclusively locked by another process.
	deployment := config.NewDeploymentDir(t.TempDir())
	mutex, err := directorymutex.New(deployment.Root())
	if err != nil {
		t.Fatalf("expected mutex creation to succeed, got: %v", err)
	}
	if err := mutex.AcquireExclusive(context.Background()); err != nil {
		t.Fatalf("expected exclusive lock to succeed, got: %v", err)
	}
	t.Cleanup(func() {
		_ = mutex.ReleaseExclusive(context.Background())
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Then: calling Stop is silently skipped without error.
	if err := Stop(ctx, deployment, false); err != nil {
		t.Fatalf("expected a locked deployment to be silently skipped, got %v", err)
	}
}

func TestWorkflowStatePermitsStop_GuidesForInitializedDeployment(t *testing.T) {
	t.Parallel()

	// Given: a deployment that has not been deployed yet.
	exasolState := &config.ExasolPersonalState{}
	if err := exasolState.SetWorkflowState(&config.WorkflowStateInitialized{}); err != nil {
		t.Fatalf("set workflow state failed: %v", err)
	}

	// When: stop permission is checked.
	decision, err := workflowStatePermitsStop(exasolState)
	// Then: the caller gets guidance instead of an unexpected-status error.
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if decision.shouldRun {
		t.Fatal("expected initialized deployment to skip backend stop")
	}
	for _, expected := range []string{"nothing to stop", "exasol deploy"} {
		if !strings.Contains(decision.guidance, expected) {
			t.Fatalf("expected guidance to contain %q, got %q", expected, decision.guidance)
		}
	}
}
