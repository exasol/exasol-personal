// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"time"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

func TestValidateCleanupRunSelectionRejectsAmbiguousTargeting(t *testing.T) {
	originalOpts := cleanupOpts
	originalRunOpts := cleanupRunOpts
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		cleanupRunOpts = originalRunOpts
	})

	// Given the two ways of selecting deployments
	// When ids and --all are combined, or neither is given
	// Then the run is rejected before anything is searched
	cleanupRunOpts.All = true
	if err := validateCleanupRunSelection([]string{"exasol-12345678"}); err == nil {
		t.Fatal("--all combined with deployment ids should be rejected")
	}

	cleanupRunOpts.All = false
	if err := validateCleanupRunSelection(nil); err == nil {
		t.Fatal("a run without ids and without --all should be rejected")
	}

	// And --older-than only narrows what --all selects, so it needs --all
	cleanupOpts.OlderThan = 24 * time.Hour
	if err := validateCleanupRunSelection([]string{"exasol-12345678"}); err == nil {
		t.Fatal("--older-than without --all should be rejected")
	}

	cleanupRunOpts.All = true
	if err := validateCleanupRunSelection(nil); err != nil {
		t.Fatalf("--all with --older-than should be accepted, got %v", err)
	}
}

func TestSelectCleanupRunDeploymentsAppliesAgeFilterOnlyForAll(t *testing.T) {
	originalOpts := cleanupOpts
	originalRunOpts := cleanupRunOpts
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		cleanupRunOpts = originalRunOpts
	})

	index := cleanupLookupIndex{Matches: map[string][]cleanupLookupMatch{
		"exasol-stale": {{Summary: shared.DeploymentSummary{CreatedAt: time.Now().Add(-72 * time.Hour)}}},
		"exasol-fresh": {{Summary: shared.DeploymentSummary{CreatedAt: time.Now()}}},
	}}

	// Given explicit ids
	// When --all is not set
	// Then the ids are used as given, regardless of their age
	cleanupRunOpts.All = false
	cleanupOpts.OlderThan = 0
	selected := selectCleanupRunDeployments([]string{"exasol-fresh"}, index)
	if len(selected) != 1 || selected[0] != "exasol-fresh" {
		t.Fatalf("selected = %v, want [exasol-fresh]", selected)
	}

	// Given --all together with an age threshold
	// When deployments are selected
	// Then only the ones past the threshold are cleaned up
	cleanupRunOpts.All = true
	cleanupOpts.OlderThan = 24 * time.Hour
	selected = selectCleanupRunDeployments(nil, index)
	if len(selected) != 1 || selected[0] != "exasol-stale" {
		t.Fatalf("selected = %v, want [exasol-stale]", selected)
	}
}

func TestCleanupRunOutcomeReportsResourceFailures(t *testing.T) {
	t.Parallel()

	// Given the tallies of a finished run
	// When resource actions failed but every deployment was processed
	// Then the run still reports an error, so the exit code alone is enough
	code, err := cleanupRunOutcome(0, 2)
	if err == nil || code != "resource_actions_failed" {
		t.Fatalf("code = %q, err = %v, want resource_actions_failed", code, err)
	}

	if code, err = cleanupRunOutcome(1, 0); err == nil || code != "deployment_requests_failed" {
		t.Fatalf("code = %q, err = %v, want deployment_requests_failed", code, err)
	}

	if code, err = cleanupRunOutcome(1, 2); err == nil || code != "run_failed" {
		t.Fatalf("code = %q, err = %v, want run_failed", code, err)
	}

	if code, err = cleanupRunOutcome(0, 0); err != nil || code != "" {
		t.Fatalf("code = %q, err = %v, want no error", code, err)
	}
}

func TestCountFailedResultsIgnoresSkippedActions(t *testing.T) {
	t.Parallel()

	results := []shared.Result{
		{Status: shared.ResultStatusSuccess},
		{Status: shared.ResultStatusSkipped},
		{Status: shared.ResultStatusFailed},
		{Status: shared.ResultStatusFailed},
	}

	// Given a mix of action outcomes
	// When failures are counted
	// Then skipped actions do not count: they are normal for protected resources
	if got := countFailedResults(results); got != 2 {
		t.Fatalf("countFailedResults = %d, want 2", got)
	}
}
