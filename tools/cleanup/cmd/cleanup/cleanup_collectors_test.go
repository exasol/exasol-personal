// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

type stubProviderCollector struct {
	name           string
	accountInfo    string
	accountInfoErr error
}

func (s stubProviderCollector) Name() string {
	return s.name
}

func (stubProviderCollector) CollectDeploymentSummaries(context.Context) ([]shared.DeploymentSummary, error) {
	return nil, nil
}

func (stubProviderCollector) IsAvailable(context.Context) bool {
	return true
}

func (s stubProviderCollector) GetAccountInfo(context.Context) (string, error) {
	if s.accountInfoErr != nil {
		return "", s.accountInfoErr
	}

	return s.accountInfo, nil
}

func (stubProviderCollector) CollectDeploymentDetails(
	context.Context,
	string,
) (*shared.DeploymentDetails, error) {
	return nil, nil
}

func (stubProviderCollector) PlanActions(
	*shared.DeploymentDetails,
	[]shared.ResourceType,
) ([]shared.Action, error) {
	return nil, nil
}

func (stubProviderCollector) ExecuteActions(
	context.Context,
	[]shared.Action,
	bool,
) ([]shared.Result, error) {
	return nil, nil
}

func TestBuildCleanupPlanUsesCallerOwnerOnlyForAWS(t *testing.T) {
	originalOpts := cleanupOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalStackitCollectorFactory := stackitCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		stackitCollectorFactory = originalStackitCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	cleanupOpts.Providers = []string{"aws", "exoscale"}
	cleanupOpts.ExoscaleZones = []string{"ch-gva-2"}

	var awsOwnerFilter string
	var exoscaleOwnerFilter string
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector {
		if region != "us-east-1" {
			t.Fatalf("aws region = %q, want us-east-1", region)
		}
		awsOwnerFilter = ownerFilter
		return stubProviderCollector{name: "aws", accountInfo: "123456789012"}
	}
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		if zone != "ch-gva-2" {
			t.Fatalf("exoscale zone = %q, want ch-gva-2", zone)
		}
		exoscaleOwnerFilter = ownerFilter
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	stackitCollectorFactory = func(projectID, region, ownerFilter string) shared.ProviderCollector {
		t.Fatalf("stackit collector should not be built when provider is not selected")
		return nil
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	// Given a command without an explicit owner filter
	// When the cleanup plan is built
	plan := buildCleanupPlan(context.Background(), false)

	// Then AWS defaults to the caller while Exoscale stays unfiltered
	if len(plan.Collectors) != 2 {
		t.Fatalf("collectors = %d, want 2", len(plan.Collectors))
	}
	if got := plan.Scope.Providers[0].Owner; got != "(caller)" {
		t.Fatalf("aws owner display = %q, want (caller)", got)
	}
	if awsOwnerFilter != "arn:aws:iam::123456789012:role/example" {
		t.Fatalf("aws owner filter = %q, want caller ARN", awsOwnerFilter)
	}
	if exoscaleOwnerFilter != "" {
		t.Fatalf("exoscale owner filter = %q, want empty", exoscaleOwnerFilter)
	}
}

func TestBuildCleanupPlanMarksUnavailableProvider(t *testing.T) {
	originalOpts := cleanupOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		awsCollectorFactory = originalAWSCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	cleanupOpts.Providers = []string{"aws"}

	var awsOwnerFilter string
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector {
		awsOwnerFilter = ownerFilter
		return stubProviderCollector{name: "aws", accountInfoErr: errors.New("missing credentials")}
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	// Given a selected provider with broken credentials
	// When the cleanup plan is built
	plan := buildCleanupPlan(context.Background(), false)

	// Then the scope reports the provider as unavailable and skips searching it
	if len(plan.Collectors) != 0 {
		t.Fatalf("collectors = %d, want 0", len(plan.Collectors))
	}
	if awsOwnerFilter != "arn:aws:iam::123456789012:role/example" {
		t.Fatalf("aws owner filter = %q, want caller ARN", awsOwnerFilter)
	}
	if plan.Scope.Providers[0].Status != providerStatusUnavailable {
		t.Fatalf("status = %q, want %q", plan.Scope.Providers[0].Status, providerStatusUnavailable)
	}
	if plan.Scope.Providers[0].Reason != "missing credentials" {
		t.Fatalf("reason = %q, want missing credentials", plan.Scope.Providers[0].Reason)
	}
}

func TestBuildCleanupPlanIncludesUnselectedProviders(t *testing.T) {
	originalOpts := cleanupOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalStackitCollectorFactory := stackitCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		stackitCollectorFactory = originalStackitCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	cleanupOpts.Providers = []string{"exoscale"}
	cleanupOpts.ExoscaleZones = []string{"ch-gva-2"}
	cleanupOpts.OwnerFilter = "owner-*"

	var awsOwnerFilter string
	var exoscaleOwnerFilter string
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector {
		awsOwnerFilter = ownerFilter
		return stubProviderCollector{name: "aws", accountInfo: "123456789012"}
	}
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		exoscaleOwnerFilter = ownerFilter
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	stackitCollectorFactory = func(projectID, region, ownerFilter string) shared.ProviderCollector {
		t.Fatalf("stackit collector should not be built when provider is not selected")
		return nil
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	// Given only Exoscale is selected with an explicit owner filter
	// When the cleanup plan is built
	plan := buildCleanupPlan(context.Background(), false)

	// Then selected and unselected providers are both present in scope
	if len(plan.Scope.Providers) != 3 {
		t.Fatalf("providers = %d, want 3", len(plan.Scope.Providers))
	}
	if exoscaleOwnerFilter != "owner-*" {
		t.Fatalf("exoscale owner filter = %q, want owner-*", exoscaleOwnerFilter)
	}
	if awsOwnerFilter != "" {
		t.Fatalf("aws owner filter = %q, want empty because AWS was not selected", awsOwnerFilter)
	}
	if plan.Scope.Providers[0].Status != providerStatusSkipped {
		t.Fatalf("aws status = %q, want %q", plan.Scope.Providers[0].Status, providerStatusSkipped)
	}
	if plan.Scope.Providers[0].Reason != providerReasonNotSelected {
		t.Fatalf("aws reason = %q, want %q", plan.Scope.Providers[0].Reason, providerReasonNotSelected)
	}
	if plan.Scope.Providers[1].Status != providerStatusSearched {
		t.Fatalf("exoscale status = %q, want %q", plan.Scope.Providers[1].Status, providerStatusSearched)
	}
	if plan.Scope.Providers[1].Reason != "" {
		t.Fatalf("exoscale reason = %q, want empty", plan.Scope.Providers[1].Reason)
	}
	if plan.Scope.Providers[2].Status != providerStatusSkipped {
		t.Fatalf("stackit status = %q, want %q", plan.Scope.Providers[2].Status, providerStatusSkipped)
	}
}

func TestBuildCleanupPlanCreatesRowPerLocationTarget(t *testing.T) {
	originalOpts := cleanupOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalStackitCollectorFactory := stackitCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		stackitCollectorFactory = originalStackitCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	cleanupOpts.Providers = []string{"aws"}
	cleanupOpts.AWSRegions = []string{"us-east-1", "eu-central-1"}

	locations := make([]string, 0)
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector {
		locations = append(locations, region)
		return stubProviderCollector{name: "aws", accountInfo: "123456789012"}
	}
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	stackitCollectorFactory = func(projectID, region, ownerFilter string) shared.ProviderCollector {
		t.Fatalf("stackit collector should not be built when provider is not selected")
		return nil
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	plan := buildCleanupPlan(context.Background(), false)

	if len(plan.Collectors) != 2 {
		t.Fatalf("collectors = %d, want 2", len(plan.Collectors))
	}
	if len(plan.Scope.Providers) != 4 {
		t.Fatalf("scope rows = %d, want 4", len(plan.Scope.Providers))
	}
	if locations[0] != "us-east-1" || locations[1] != "eu-central-1" {
		t.Fatalf("locations = %v, want [us-east-1 eu-central-1]", locations)
	}
	if plan.Scope.Providers[0].Location != "us-east-1" || plan.Scope.Providers[1].Location != "eu-central-1" {
		t.Fatalf("aws scope locations = %q,%q", plan.Scope.Providers[0].Location, plan.Scope.Providers[1].Location)
	}
	if plan.Scope.Providers[2].Status != providerStatusSkipped {
		t.Fatalf("exoscale status = %q, want %q", plan.Scope.Providers[2].Status, providerStatusSkipped)
	}
	if plan.Scope.Providers[3].Status != providerStatusSkipped {
		t.Fatalf("stackit status = %q, want %q", plan.Scope.Providers[3].Status, providerStatusSkipped)
	}
}

func TestBuildCleanupPlanIncludesStackitTarget(t *testing.T) {
	originalOpts := cleanupOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalStackitCollectorFactory := stackitCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		stackitCollectorFactory = originalStackitCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	cleanupOpts.Providers = []string{"stackit"}
	cleanupOpts.StackitProjectID = "project-1"
	cleanupOpts.StackitRegions = []string{"eu01", "eu02"}
	cleanupOpts.OwnerFilter = "owner-*"

	stackitCalls := make([]string, 0)
	stackitCollectorFactory = func(projectID, region, ownerFilter string) shared.ProviderCollector {
		if projectID != "project-1" {
			t.Fatalf("stackit project id = %q, want project-1", projectID)
		}
		if ownerFilter != "owner-*" {
			t.Fatalf("stackit owner filter = %q, want owner-*", ownerFilter)
		}
		stackitCalls = append(stackitCalls, region)
		return stubProviderCollector{name: "stackit", accountInfo: "project-name"}
	}
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector {
		t.Fatalf("aws collector should not be built when provider is not selected")
		return nil
	}
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		t.Fatalf("exoscale collector should not be built when provider is not selected")
		return nil
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "", false
	}

	// Given only Stackit is selected across two regions
	// When the cleanup plan is built
	plan := buildCleanupPlan(context.Background(), false)

	// Then both Stackit targets are searched and unselected providers remain visible in scope
	if len(plan.Collectors) != 2 {
		t.Fatalf("collectors = %d, want 2", len(plan.Collectors))
	}
	if len(stackitCalls) != 2 || stackitCalls[0] != "eu01" || stackitCalls[1] != "eu02" {
		t.Fatalf("stackit calls = %v, want [eu01 eu02]", stackitCalls)
	}
	if len(plan.Scope.Providers) != 4 {
		t.Fatalf("scope rows = %d, want 4", len(plan.Scope.Providers))
	}
	if plan.Scope.Providers[2].Provider != "stackit" || plan.Scope.Providers[2].Status != providerStatusSearched {
		t.Fatalf("first stackit scope row = %#v, want searched", plan.Scope.Providers[2])
	}
	if plan.Scope.Providers[3].Provider != "stackit" || plan.Scope.Providers[3].Status != providerStatusSearched {
		t.Fatalf("second stackit scope row = %#v, want searched", plan.Scope.Providers[3])
	}
}

func TestBuildCleanupPlanMarksStackitMissingProjectUnavailable(t *testing.T) {
	originalOpts := cleanupOpts
	originalStackitCollectorFactory := stackitCollectorFactory
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		stackitCollectorFactory = originalStackitCollectorFactory
	})

	cleanupOpts.Providers = []string{"stackit"}
	stackitCollectorFactory = func(projectID, region, ownerFilter string) shared.ProviderCollector {
		t.Fatalf("stackit collector should not be built without project id")
		return nil
	}

	// Given Stackit is selected without the required project id
	// When the cleanup plan is built
	plan := buildCleanupPlan(context.Background(), false)

	// Then Stackit appears in scope as unavailable instead of failing flag parsing globally
	if len(plan.Collectors) != 0 {
		t.Fatalf("collectors = %d, want 0", len(plan.Collectors))
	}
	stackitScope := plan.Scope.Providers[2]
	if stackitScope.Provider != "stackit" {
		t.Fatalf("provider = %q, want stackit", stackitScope.Provider)
	}
	if stackitScope.Status != providerStatusUnavailable {
		t.Fatalf("status = %q, want %q", stackitScope.Status, providerStatusUnavailable)
	}
	if stackitScope.Reason == "" {
		t.Fatal("expected missing project reason")
	}
}
