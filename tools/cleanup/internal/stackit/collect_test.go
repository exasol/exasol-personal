// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"testing"
	"time"
)

func TestSummarizeDeploymentResourcesUsesEarliestTimestampAndActiveState(t *testing.T) {
	t.Parallel()

	createdLater := time.Date(2026, time.January, 2, 10, 0, 0, 0, time.UTC)
	createdEarlier := time.Date(2026, time.January, 1, 8, 30, 0, 0, time.UTC)

	resources := []ResourceMeta{
		{
			Ref:  ResourceRef{Type: ResourceServer},
			Tags: map[string]string{"owner": "qa-team"},
			Attr: map[string]any{
				"name":      "exasol-251a4801-node-1",
				"state":     StateActive,
				"createdAt": createdLater,
			},
		},
		{
			Ref:  ResourceRef{Type: ResourceVolume},
			Tags: map[string]string{},
			Attr: map[string]any{
				"name":      "exasol-251a4801-volume",
				"createdAt": createdEarlier,
			},
		},
	}

	// Given a deployment with multiple resources and mixed timestamps
	// When the summary is derived from the resource list
	summary := summarizeDeploymentResources("exasol-251a4801", "eu01", resources)

	// Then it keeps the earliest timestamp and marks the deployment active
	if summary.CreatedAt != createdEarlier {
		t.Fatalf("expected earliest createdAt %v, got %v", createdEarlier, summary.CreatedAt)
	}
	if summary.State != StateActive {
		t.Fatalf("expected state %q, got %q", StateActive, summary.State)
	}
	if summary.Owner != "qa-team" {
		t.Fatalf("expected owner %q, got %q", "qa-team", summary.Owner)
	}
	if summary.Resources != len(resources) {
		t.Fatalf("expected %d resources, got %d", len(resources), summary.Resources)
	}
}

func TestSummarizeDeploymentResourcesMarksProvisioningServers(t *testing.T) {
	t.Parallel()

	resources := []ResourceMeta{
		{
			Ref:  ResourceRef{Type: ResourceServer},
			Tags: map[string]string{},
			Attr: map[string]any{
				"name":  "exasol-251a4801-node-1",
				"state": StateProvisioning,
			},
		},
	}

	// Given a deployment with a server still coming up
	// When the summary is derived from the resource list
	summary := summarizeDeploymentResources("exasol-251a4801", "eu01", resources)

	// Then the deployment stays in provisioning instead of looking terminated
	if summary.State != StateProvisioning {
		t.Fatalf("expected state %q, got %q", StateProvisioning, summary.State)
	}
	if summary.Owner != "-" {
		t.Fatalf("expected fallback owner %q, got %q", "-", summary.Owner)
	}

}

func TestSummarizeDeploymentResourcesMarksOrphanedWithoutServers(t *testing.T) {
	t.Parallel()

	resources := []ResourceMeta{
		{
			Ref:  ResourceRef{Type: ResourceObjectStorageBucket},
			Tags: map[string]string{},
			Attr: map[string]any{"name": "exasol-251a4801"},
		},
	}

	// Given a deployment that only has non-server resources left
	// When the summary is derived from the resource list
	summary := summarizeDeploymentResources("exasol-251a4801", "eu01", resources)

	// Then it is reported as orphaned and no panic occurs
	if summary.State != "orphaned" {
		t.Fatalf("expected state %q, got %q", "orphaned", summary.State)
	}
	if !summary.CreatedAt.IsZero() {
		t.Fatalf("expected zero createdAt, got %v", summary.CreatedAt)
	}
}
