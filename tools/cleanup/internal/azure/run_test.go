// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

func sampleDetails() *DeploymentDetails {
	return &DeploymentDetails{
		Resources: []ResourceMeta{
			{Ref: ResourceRef{Type: ResourceResourceGroup, ID: "exasol-12345678-rg"}},
			{Ref: ResourceRef{Type: ResourceVirtualMachine, ID: "exasol-12345678-n11"}},
			{Ref: ResourceRef{Type: ResourceDisk, ID: "exasol-12345678-n11-data"}},
		},
	}
}

func TestPlanActionsCoversResourcesAndDeletesGroupLast(t *testing.T) {
	t.Parallel()

	actions, err := PlanActions(sampleDetails(), nil)
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("actions = %d, want 3", len(actions))
	}

	// Contained resources are covered by the cascade, not deleted individually.
	for _, action := range actions[:2] {
		if action.Op != OpSkip || action.Reason != reasonCascade {
			t.Fatalf("contained action = %+v, want skip/%q", action, reasonCascade)
		}
	}

	// The resource-group deletion is the single executed operation, ordered last.
	last := actions[len(actions)-1]
	if last.Ref.Type != ResourceResourceGroup || last.Op != OpDelete {
		t.Fatalf("last action = %+v, want resource-group delete", last)
	}
}

func TestPlanActionsTypeFilterExcludingGroupOmitsDeletion(t *testing.T) {
	t.Parallel()

	actions, err := PlanActions(sampleDetails(), []ResourceType{ResourceVirtualMachine})
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(actions))
	}
	if actions[0].Ref.Type != ResourceVirtualMachine || actions[0].Op != OpSkip {
		t.Fatalf("action = %+v, want vm skip", actions[0])
	}
}

func TestPlanActionsTypeFilterSelectsGroupDeletion(t *testing.T) {
	t.Parallel()

	actions, err := PlanActions(sampleDetails(), []ResourceType{ResourceResourceGroup})
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(actions))
	}
	if actions[0].Ref.Type != ResourceResourceGroup || actions[0].Op != OpDelete {
		t.Fatalf("action = %+v, want resource-group delete", actions[0])
	}
}

func TestPlanActionsEmptyReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := PlanActions(&DeploymentDetails{}, nil); !errors.Is(err, ErrNoResourcesPlanned) {
		t.Fatalf("err = %v, want ErrNoResourcesPlanned", err)
	}
}

func TestExecuteActionsDryRunDoesNotDelete(t *testing.T) {
	t.Parallel()

	actions := []Action{
		{Ref: ResourceRef{Type: ResourceVirtualMachine, ID: "vm"}, Op: OpSkip, Reason: reasonCascade},
		{Ref: ResourceRef{Type: ResourceResourceGroup, ID: "exasol-12345678-rg"}, Op: OpDelete},
	}

	results, err := ExecuteActions(context.Background(), "sub", actions, false)
	if err != nil {
		t.Fatalf("ExecuteActions returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].Status != "skipped" {
		t.Fatalf("contained status = %q, want skipped", results[0].Status)
	}
	if results[1].Status != "planned" {
		t.Fatalf("resource-group status = %q, want planned (dry-run)", results[1].Status)
	}
}

// ensure the shared alias wiring stays intact.
var _ shared.ProviderCollector = (*Collector)(nil)
