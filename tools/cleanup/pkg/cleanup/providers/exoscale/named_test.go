// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"errors"
	"testing"
	"time"
)

func noDeploymentsElsewhere(context.Context) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func TestAddOrganizationResourcesDiscoversOrphanedDeployments(t *testing.T) {
	// Given the security group, SSH key, IAM role, and IAM API key of a deployment without zonal resources
	created := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	resources := []namedResource{
		{name: "exasol-e5b1becf-sg"},
		{name: "exasol-e5b1becf-key"},
		{name: "exasol-e5b1becf-archive-sos"},
		{name: "exasol-e5b1becf-archive-sos-key", createdAt: created},
		{name: "GitHub CI", createdAt: created},
	}
	summaries := map[string]*DeploymentSummary{}

	// When
	addOrganizationResourcesToSummaries(
		context.Background(), resources, "ch-gva-2", "*", summaries, noDeploymentsElsewhere,
	)

	// Then the deployment is found with all four resources and the API key's creation time
	sum, ok := summaries["exasol-e5b1becf"]
	if !ok || len(summaries) != 1 {
		t.Fatalf("got deployments %v, want only exasol-e5b1becf", summaries)
	}
	if sum.Resources != 4 {
		t.Errorf("Resources = %d, want 4", sum.Resources)
	}
	if !sum.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", sum.CreatedAt, created)
	}
	if !sum.LocationIndependent {
		t.Error("LocationIndependent = false, want true")
	}
}

func TestAddOrganizationResourcesCountsKnownDeployments(t *testing.T) {
	// Given a deployment that a zonal resource already established
	summaries := map[string]*DeploymentSummary{
		"exasol-0badc0de": {ID: "exasol-0badc0de", Owner: "alice", Resources: 1},
	}
	lookups := 0
	elsewhere := func(context.Context) (map[string]bool, error) {
		lookups++
		return map[string]bool{}, nil
	}

	// When its security group is listed under a specific owner filter
	addOrganizationResourcesToSummaries(
		context.Background(), []namedResource{{name: "exasol-0badc0de-sg"}}, "ch-gva-2", "alice", summaries, elsewhere,
	)

	// Then the security group counts toward it without looking at other zones
	if got := summaries["exasol-0badc0de"].Resources; got != 2 {
		t.Errorf("Resources = %d, want 2", got)
	}
	if lookups != 0 {
		t.Errorf("other zones were searched %d times, want 0", lookups)
	}
	if summaries["exasol-0badc0de"].LocationIndependent {
		t.Error("LocationIndependent = true, want false for a deployment with zonal resources")
	}
}

func TestAddOrganizationResourcesSkipsDeploymentsInOtherZones(t *testing.T) {
	// Given organization-wide resources of a deployment that still has instances in another zone
	elsewhere := func(context.Context) (map[string]bool, error) {
		return map[string]bool{"exasol-9d12060b": true}, nil
	}
	summaries := map[string]*DeploymentSummary{}

	// When
	addOrganizationResourcesToSummaries(
		context.Background(),
		[]namedResource{{name: "exasol-9d12060b-sg"}, {name: "exasol-9d12060b-key"}},
		"ch-gva-2", "*", summaries, elsewhere,
	)

	// Then the deployment is not reported in this zone
	if len(summaries) != 0 {
		t.Errorf("got deployments %v, want none", summaries)
	}
}

func TestAddOrganizationResourcesSkipsOrphansWhenOtherZonesAreUnknown(t *testing.T) {
	// Given that the other zones cannot be searched
	elsewhere := func(context.Context) (map[string]bool, error) {
		return nil, errors.New("zone unavailable")
	}
	summaries := map[string]*DeploymentSummary{}

	// When
	addOrganizationResourcesToSummaries(
		context.Background(), []namedResource{{name: "exasol-9d12060b-sg"}}, "ch-gva-2", "*", summaries, elsewhere,
	)

	// Then no deployment is established from organization-wide resources
	if len(summaries) != 0 {
		t.Errorf("got deployments %v, want none", summaries)
	}
}

func TestAddOrganizationResourcesRespectsOwnerFilter(t *testing.T) {
	// Given organization-wide resources of an unknown deployment
	summaries := map[string]*DeploymentSummary{}

	// When the owner filter names a specific owner
	addOrganizationResourcesToSummaries(
		context.Background(), []namedResource{{name: "exasol-9d12060b-sg"}}, "ch-gva-2", "alice", summaries,
		noDeploymentsElsewhere,
	)

	// Then the ownerless resources do not establish a deployment
	if len(summaries) != 0 {
		t.Errorf("got deployments %v, want none", summaries)
	}
}
