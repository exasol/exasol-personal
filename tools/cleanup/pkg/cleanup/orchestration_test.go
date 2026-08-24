// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package cleanup

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubCollector struct {
	name           string
	accountInfo    string
	accountInfoErr error
	summaries      []DeploymentSummary
	summaryErr     error
}

func (s stubCollector) Name() string {
	return s.name
}

func (s stubCollector) CollectDeploymentSummaries(context.Context) ([]DeploymentSummary, error) {
	return s.summaries, s.summaryErr
}

func (stubCollector) IsAvailable(context.Context) bool {
	return true
}

func (s stubCollector) GetAccountInfo(context.Context) (string, error) {
	if s.accountInfoErr != nil {
		return "", s.accountInfoErr
	}

	return s.accountInfo, nil
}

func (stubCollector) CollectDeploymentDetails(context.Context, string) (*DeploymentDetails, error) {
	return nil, nil
}

func (stubCollector) PlanActions(*DeploymentDetails, []ResourceType) ([]Action, error) {
	return nil, nil
}

func (stubCollector) ExecuteActions(context.Context, []Action, bool) ([]Result, error) {
	return nil, nil
}

func TestBuildPlanReturnsTypedScopeWithoutTerminalOutput(t *testing.T) {
	// Given provider specs with one selected provider and one unavailable selected provider
	specs := []ProviderSpec{
		{
			Name:      "aws",
			Locations: func() []string { return []string{"eu-central-1"} },
			ResolveOwner: func(context.Context, string) OwnerResolution {
				return OwnerResolution{Filter: "owner-a", Display: "owner-a", Source: "explicit"}
			},
			BuildCollector: func(string, string, bool) ProviderCollector {
				return stubCollector{name: "aws", accountInfo: "account-a"}
			},
		},
		{
			Name:      "azure",
			Locations: func() []string { return []string{"westeurope"} },
			ResolveOwner: func(context.Context, string) OwnerResolution {
				return OwnerResolution{Display: "*", Source: "default", Err: errors.New("missing subscription")}
			},
			BuildCollector: func(string, string, bool) ProviderCollector {
				t.Fatal("collector should not be built when owner resolution failed")
				return nil
			},
		},
	}

	// When a library plan is built
	plan := BuildPlan(context.Background(), specs, []string{"aws", "azure"}, false)

	// Then callers receive typed scope and collector data to render themselves
	if len(plan.Collectors) != 1 {
		t.Fatalf("collectors = %d, want 1", len(plan.Collectors))
	}
	if plan.Scope.Providers[0].Status != ProviderStatusSearched {
		t.Fatalf("aws status = %q, want %q", plan.Scope.Providers[0].Status, ProviderStatusSearched)
	}
	if plan.Scope.Providers[1].Status != ProviderStatusUnavailable {
		t.Fatalf("azure status = %q, want %q", plan.Scope.Providers[1].Status, ProviderStatusUnavailable)
	}
	if plan.Scope.Providers[1].Reason != "missing subscription" {
		t.Fatalf("azure reason = %q, want missing subscription", plan.Scope.Providers[1].Reason)
	}
}

func TestCollectLookupIndexPreservesProviderFailures(t *testing.T) {
	// Given a plan with one successful provider and one provider that fails discovery
	successScope := ScopeProvider{Provider: "aws", Location: "eu-central-1", Status: ProviderStatusSearched}
	failedScope := ScopeProvider{Provider: "exoscale", Location: "ch-gva-2", Status: ProviderStatusSearched}
	plan := Plan{
		Scope: Scope{Providers: []ScopeProvider{successScope, failedScope}},
		Collectors: []ScopedCollector{
			{
				Collector: stubCollector{
					name:        "aws",
					accountInfo: "account-a",
					summaries: []DeploymentSummary{
						{ID: "dep-a", Provider: "aws", Region: "eu-central-1", Owner: "alice"},
					},
				},
				Scope: &successScope,
			},
			{
				Collector: stubCollector{name: "exoscale", summaryErr: errors.New("provider failed")},
				Scope:     &failedScope,
			},
		},
	}

	// When callers build a deployment lookup index
	index := CollectLookupIndex(context.Background(), &plan)

	// Then successful matches and provider-level failure details are both inspectable
	if index.SuccessCount != 1 {
		t.Fatalf("success count = %d, want 1", index.SuccessCount)
	}
	if got := index.Matches["dep-a"][0].Resolved.Location; got != "eu-central-1" {
		t.Fatalf("resolved location = %q, want eu-central-1", got)
	}
	if failedScope.Status != ProviderStatusError {
		t.Fatalf("failed status = %q, want %q", failedScope.Status, ProviderStatusError)
	}
	if failedScope.Reason != "provider failed" {
		t.Fatalf("failed reason = %q, want provider failed", failedScope.Reason)
	}
}

func TestFilterDeploymentDetailsByTypesReturnsTypedCopy(t *testing.T) {
	createdAt := time.Unix(10, 0)
	details := &DeploymentDetails{
		Summary: DeploymentSummary{ID: "dep-a", CreatedAt: createdAt, Resources: 2},
		Resources: []ResourceMeta{
			{Ref: ResourceRef{Type: "ec2-instance", ID: "i-a"}},
			{Ref: ResourceRef{Type: "ebs-volume", ID: "vol-a"}},
		},
	}

	filtered := FilterDeploymentDetailsByTypes(details, []ResourceType{"ebs-volume"})

	if len(filtered.Resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(filtered.Resources))
	}
	if filtered.Resources[0].Ref.ID != "vol-a" {
		t.Fatalf("resource id = %q, want vol-a", filtered.Resources[0].Ref.ID)
	}
	if filtered.Summary.Resources != 1 {
		t.Fatalf("summary resources = %d, want 1", filtered.Summary.Resources)
	}
	if details.Summary.Resources != 2 {
		t.Fatalf("original summary resources = %d, want unchanged 2", details.Summary.Resources)
	}
}

func TestIsOlderThanTreatsUndatedDeploymentsAsStale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	// Given a deployment age threshold
	// When the creation time is unknown, recent, or well past the threshold
	// Then only the recent deployment survives, and an unknown time counts as stale
	if !IsOlderThan(time.Time{}, time.Hour, now) {
		t.Fatal("deployment without a creation time should count as stale")
	}
	if IsOlderThan(now.Add(-30*time.Minute), time.Hour, now) {
		t.Fatal("deployment younger than the threshold should not count as stale")
	}
	if !IsOlderThan(now.Add(-2*time.Hour), time.Hour, now) {
		t.Fatal("deployment older than the threshold should count as stale")
	}
	if !IsOlderThan(now, 0, now) {
		t.Fatal("a zero threshold should select every deployment")
	}
}

func TestFilterSummariesOlderThanKeepsOnlyStaleDeployments(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	summaries := []DeploymentSummary{
		{ID: "dep-fresh", CreatedAt: now.Add(-time.Hour)},
		{ID: "dep-stale", CreatedAt: now.Add(-48 * time.Hour)},
	}

	// Given deployments of differing age
	// When a threshold is applied
	// Then only the deployments past it remain, and no threshold keeps them all
	filtered := FilterSummariesOlderThan(summaries, 24*time.Hour, now)
	if len(filtered) != 1 || filtered[0].ID != "dep-stale" {
		t.Fatalf("filtered = %v, want [dep-stale]", filtered)
	}

	if got := FilterSummariesOlderThan(summaries, 0, now); len(got) != 2 {
		t.Fatalf("unfiltered = %v, want both summaries", got)
	}
}

func TestStaleDeploymentIDsSortsAndDeduplicatesLocations(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	index := LookupIndex{Matches: map[string][]LookupMatch{
		"dep-b": {{Summary: DeploymentSummary{ID: "dep-b", CreatedAt: now.Add(-48 * time.Hour)}}},
		"dep-a": {
			{Summary: DeploymentSummary{ID: "dep-a", CreatedAt: now.Add(-time.Minute)}},
			{Summary: DeploymentSummary{ID: "dep-a", CreatedAt: now.Add(-72 * time.Hour)}},
		},
		"dep-fresh": {{Summary: DeploymentSummary{ID: "dep-fresh", CreatedAt: now}}},
	}}

	// Given deployments found once or in several locations
	// When stale ids are selected
	// Then each id appears once, sorted, and one stale location is enough
	ids := StaleDeploymentIDs(index, 24*time.Hour, now)
	if len(ids) != 2 || ids[0] != "dep-a" || ids[1] != "dep-b" {
		t.Fatalf("ids = %v, want [dep-a dep-b]", ids)
	}
}
