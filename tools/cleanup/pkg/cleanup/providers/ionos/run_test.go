// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"context"
	"errors"
	"testing"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

func refs(actions []Action) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.Op+":"+action.Ref.ID)
	}

	return out
}

func assertRefs(t *testing.T, actions []Action, want ...string) {
	t.Helper()
	got := refs(actions)
	if len(got) != len(want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("actions = %v, want %v", got, want)
		}
	}
}

func TestPlanActionsDeletesOwnedDatacenterThenIPBlock(t *testing.T) {
	t.Parallel()

	// Given a deployment that owns its datacenter
	details := groupDeployments(sampleInventory(), DefaultLocation)["exasol-aaaaaaaa"]

	// When cleanup actions are planned
	actions, err := PlanActions(details, nil)
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}

	// Then the datacenter takes its contents with it and the IP block goes last
	assertRefs(t, actions, "skip:srv-1", "skip:vol-1", "skip:1", "delete:dc-own", "delete:ip-a")
	if actions[0].Reason != reasonCascade {
		t.Fatalf("reason = %q, want %q", actions[0].Reason, reasonCascade)
	}
}

func TestPlanActionsKeepsForeignDatacenter(t *testing.T) {
	t.Parallel()

	// Given a deployment inside a datacenter the launcher did not create
	details := groupDeployments(sampleInventory(), DefaultLocation)["exasol-bbbbbbbb"]

	// When cleanup actions are planned
	actions, err := PlanActions(details, nil)
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}

	// Then its resources are deleted one by one and the datacenter is kept
	assertRefs(t, actions, "delete:srv-2", "delete:vol-2", "delete:ip-b")
}

func TestPlanActionsHonoursTypeFilter(t *testing.T) {
	t.Parallel()

	// Given a deployment with several resource types
	details := groupDeployments(sampleInventory(), DefaultLocation)["exasol-aaaaaaaa"]

	// When cleanup is limited to IP blocks
	actions, err := PlanActions(details, []ResourceType{ResourceIPBlock})
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}

	// Then only the IP block is planned
	assertRefs(t, actions, "delete:ip-a")
}

func TestPlanActionsDeletesSelectedChildrenOfOwnedDatacenter(t *testing.T) {
	t.Parallel()

	// Given a deployment that owns its datacenter
	details := groupDeployments(sampleInventory(), DefaultLocation)["exasol-aaaaaaaa"]

	// When cleanup is limited to servers
	actions, err := PlanActions(details, []ResourceType{ResourceServer})
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}

	// Then the server is deleted itself, since the datacenter is not deleted
	assertRefs(t, actions, "delete:srv-1")
}

func TestPlanActionsDeletesChildrenOutsideTheDeletedDatacenter(t *testing.T) {
	t.Parallel()

	// Given a deployment with its own datacenter and a server in another datacenter
	details := &DeploymentDetails{Resources: []ResourceMeta{
		{Ref: ResourceRef{Type: ResourceDatacenter, ID: "dc-own"}},
		{Ref: ResourceRef{Type: ResourceServer, ID: "srv-own", ParentID: "dc-own"}},
		{Ref: ResourceRef{Type: ResourceServer, ID: "srv-shared", ParentID: "dc-shared"}},
	}}

	// When cleanup actions are planned
	actions, err := PlanActions(details, nil)
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}

	// Then only the server inside the deleted datacenter is left to the cascade
	assertRefs(t, actions, "skip:srv-own", "delete:dc-own", "delete:srv-shared")
}

func TestPlanActionsRejectsEmptyDetails(t *testing.T) {
	t.Parallel()

	// Given a deployment without resources
	// When cleanup actions are planned
	// Then planning fails with ErrNoResourcesPlanned
	if _, err := PlanActions(&DeploymentDetails{}, nil); !errors.Is(err, ErrNoResourcesPlanned) {
		t.Fatalf("err = %v, want ErrNoResourcesPlanned", err)
	}
}

func TestExecuteActionsDryRunDoesNotDelete(t *testing.T) {
	t.Parallel()

	// Given a skip and a delete action
	actions := []Action{
		{Ref: ResourceRef{Type: ResourceServer, ID: "srv-1"}, Op: OpSkip, Reason: reasonCascade},
		{Ref: ResourceRef{Type: ResourceDatacenter, ID: "dc-own"}, Op: OpDelete},
	}

	// When they run without --execute
	results, err := ExecuteActions(context.Background(), actions, false)
	if err != nil {
		t.Fatalf("ExecuteActions returned error: %v", err)
	}

	// Then nothing is deleted and the delete is only planned
	if results[0].Status != shared.ResultStatusSkipped || results[1].Status != shared.ResultStatusPlanned {
		t.Fatalf("results = %+v, want skipped then planned", results)
	}
}
