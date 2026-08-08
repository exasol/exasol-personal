// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestNoLifecycleActionDoesNotRunOrShowInstructions(t *testing.T) {
	t.Parallel()

	decision := noLifecycleAction()

	if decision.shouldRun || decision.guidance != "" || decision.showConnectionInstructions {
		t.Fatalf("expected an inert decision, got %+v", decision)
	}
}

func TestRunLifecycleActionRunsWithoutGuidance(t *testing.T) {
	t.Parallel()

	decision := runLifecycleAction()

	if !decision.shouldRun || decision.guidance != "" {
		t.Fatalf("expected a run decision with no guidance, got %+v", decision)
	}
}

func TestSkipLifecycleActionWithConnectionInstructionsSetsFlag(t *testing.T) {
	t.Parallel()

	decision := skipLifecycleActionWithConnectionInstructions("already running")

	if decision.shouldRun || decision.guidance != "already running" ||
		!decision.showConnectionInstructions {
		t.Fatalf("expected a skip decision showing connection instructions, got %+v", decision)
	}
}

func TestOperationInProgressGuidanceMentionsBothOperations(t *testing.T) {
	t.Parallel()

	guidance := operationInProgressGuidance("deploy", "start")

	if !strings.Contains(guidance, "deploy") || !strings.Contains(guidance, "exasol start") {
		t.Fatalf("expected guidance to mention both operations, got %q", guidance)
	}
}

func TestInterruptedOperationGuidanceMentionsBothOperations(t *testing.T) {
	t.Parallel()

	guidance := interruptedOperationGuidance("start", "stop")

	if !strings.Contains(guidance, "start") || !strings.Contains(guidance, "exasol stop") {
		t.Fatalf("expected guidance to mention both operations, got %q", guidance)
	}
}

func TestLogLifecycleGuidanceIsANoopForEmptyMessage(t *testing.T) {
	t.Parallel()

	// Just verifying it doesn't panic; there's no observable output to assert.
	logLifecycleGuidance("")
	logLifecycleGuidance("some guidance")
}

func TestMarkOperationInterruptedPersistsInterruptedStateAndReturnsOriginalError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{DeploymentVersion: "0.0.0"}
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("failed to write initial state: %v", err)
	}

	originalErr := errors.New("boom")

	returnedErr := markOperationInterrupted(state, deployment, "deploy", originalErr)

	if !errors.Is(returnedErr, originalErr) {
		t.Fatalf("expected the original error to be returned, got %v", returnedErr)
	}

	persisted, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("failed to read persisted state: %v", err)
	}
	workflowState, err := persisted.GetWorkflowState()
	if err != nil {
		t.Fatalf("failed to get workflow state: %v", err)
	}
	interrupted, ok := workflowState.(*config.WorkflowStateInterrupted)
	if !ok {
		t.Fatalf("expected WorkflowStateInterrupted, got %T", workflowState)
	}
	if interrupted.InterruptedDuringOperation != "deploy" || interrupted.Error != "boom" {
		t.Fatalf("unexpected interrupted state: %+v", interrupted)
	}
}
