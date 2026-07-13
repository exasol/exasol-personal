// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
	"github.com/spf13/cobra"
)

type scenarioProviderCollector struct {
	name                string
	accountInfo         string
	summaries           []shared.DeploymentSummary
	detailsByDeployment map[string]*shared.DeploymentDetails
	resultsByDeployment map[string][]shared.Result
	planCalls           []string
	executeCalls        []scenarioExecuteCall
}

func TestCleanupShowJSONAlwaysUsesDeploymentsArray(t *testing.T) {
	originalOpts := cleanupOpts
	originalShowOpts := cleanupShowOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		cleanupShowOpts = originalShowOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	collector := &scenarioProviderCollector{
		name:        "aws",
		accountInfo: "123456789012",
		summaries:   []shared.DeploymentSummary{{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", Resources: 1}},
		detailsByDeployment: map[string]*shared.DeploymentDetails{
			"dep-a": {Summary: shared.DeploymentSummary{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", Resources: 1}},
		},
	}

	cleanupOpts.JSON = true
	cleanupOpts.Providers = []string{"aws"}
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector { return collector }
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Given one deployment id in JSON mode
	// When show runs
	err := cleanupShowCmd.RunE(cmd, []string{"dep-a"})

	// Then the response keeps the same array envelope as batch requests
	if err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if _, ok := payload["resolved"]; ok {
		t.Fatalf("payload unexpectedly contains legacy resolved field: %s", stdout.String())
	}
	if _, ok := payload["data"]; ok {
		t.Fatalf("payload unexpectedly contains legacy data field: %s", stdout.String())
	}
	deployments, ok := payload["deployments"].([]any)
	if !ok || len(deployments) != 1 {
		t.Fatalf("deployments = %#v, want one item", payload["deployments"])
	}
	deployment, ok := deployments[0].(map[string]any)
	if !ok {
		t.Fatalf("deployment item = %#v, want object", deployments[0])
	}
	if _, ok := deployment["details"]; !ok {
		t.Fatalf("deployment missing details: %s", stdout.String())
	}
}

type scenarioExecuteCall struct {
	deploymentID string
	execute      bool
}

func TestCleanupRunJSONAlwaysUsesDeploymentsArray(t *testing.T) {
	originalOpts := cleanupOpts
	originalRunOpts := cleanupRunOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		cleanupRunOpts = originalRunOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	collector := &scenarioProviderCollector{
		name:        "aws",
		accountInfo: "123456789012",
		summaries:   []shared.DeploymentSummary{{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", Resources: 1}},
		detailsByDeployment: map[string]*shared.DeploymentDetails{
			"dep-a": {Summary: shared.DeploymentSummary{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", Resources: 1}},
		},
		resultsByDeployment: map[string][]shared.Result{
			"dep-a": {{Action: shared.Action{Ref: shared.ResourceRef{Type: "ec2-instance", ID: "i-a", ARN: "dep-a"}, Op: "delete", Reason: "ordered"}, Status: "planned"}},
		},
	}

	cleanupOpts.JSON = true
	cleanupOpts.Providers = []string{"aws"}
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector { return collector }
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Given one deployment id in JSON mode
	// When run executes in dry-run mode
	err := cleanupRunCmd.RunE(cmd, []string{"dep-a"})

	// Then the response stays in the batch envelope with results under deployments
	if err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if _, ok := payload["resolved"]; ok {
		t.Fatalf("payload unexpectedly contains legacy resolved field: %s", stdout.String())
	}
	if _, ok := payload["data"]; ok {
		t.Fatalf("payload unexpectedly contains legacy data field: %s", stdout.String())
	}
	if _, ok := payload["summary"]; ok {
		t.Fatalf("payload unexpectedly contains legacy summary field: %s", stdout.String())
	}
	deployments, ok := payload["deployments"].([]any)
	if !ok || len(deployments) != 1 {
		t.Fatalf("deployments = %#v, want one item", payload["deployments"])
	}
	deployment, ok := deployments[0].(map[string]any)
	if !ok {
		t.Fatalf("deployment item = %#v, want object", deployments[0])
	}
	if _, ok := deployment["results"]; !ok {
		t.Fatalf("deployment missing results: %s", stdout.String())
	}
	if _, ok := deployment["summary"]; !ok {
		t.Fatalf("deployment missing summary: %s", stdout.String())
	}
}

func (s *scenarioProviderCollector) Name() string {
	return s.name
}

func (s *scenarioProviderCollector) CollectDeploymentSummaries(context.Context) ([]shared.DeploymentSummary, error) {
	return s.summaries, nil
}

func (*scenarioProviderCollector) IsAvailable(context.Context) bool {
	return true
}

func (s *scenarioProviderCollector) GetAccountInfo(context.Context) (string, error) {
	return s.accountInfo, nil
}

func (s *scenarioProviderCollector) CollectDeploymentDetails(
	_ context.Context,
	deploymentID string,
) (*shared.DeploymentDetails, error) {
	return s.detailsByDeployment[deploymentID], nil
}

func (s *scenarioProviderCollector) PlanActions(
	details *shared.DeploymentDetails,
	_ []shared.ResourceType,
) ([]shared.Action, error) {
	s.planCalls = append(s.planCalls, details.Summary.ID)
	results := s.resultsByDeployment[details.Summary.ID]
	actions := make([]shared.Action, 0, len(results))
	for _, result := range results {
		actions = append(actions, result.Action)
	}

	return actions, nil
}

func (s *scenarioProviderCollector) ExecuteActions(
	_ context.Context,
	actions []shared.Action,
	execute bool,
) ([]shared.Result, error) {
	deploymentID := ""
	if len(actions) > 0 {
		deploymentID = actions[0].Ref.ARN
	}
	s.executeCalls = append(s.executeCalls, scenarioExecuteCall{deploymentID: deploymentID, execute: execute})

	return s.resultsByDeployment[deploymentID], nil
}

func TestCleanupShowSupportsMultipleDeploymentIDs(t *testing.T) {
	originalOpts := cleanupOpts
	originalShowOpts := cleanupShowOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		cleanupShowOpts = originalShowOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	collector := &scenarioProviderCollector{
		name:        "aws",
		accountInfo: "123456789012",
		summaries: []shared.DeploymentSummary{
			{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", CreatedAt: time.Unix(10, 0), State: "running", Resources: 1},
			{ID: "dep-b", Provider: "aws", Region: "us-east-1", Owner: "bob", CreatedAt: time.Unix(20, 0), State: "stopped", Resources: 1},
		},
		detailsByDeployment: map[string]*shared.DeploymentDetails{
			"dep-a": {
				Summary:   shared.DeploymentSummary{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", CreatedAt: time.Unix(10, 0), State: "running", Resources: 1},
				Resources: []shared.ResourceMeta{{Ref: shared.ResourceRef{Type: "ec2-instance", ID: "i-a", ARN: "arn:a"}, Tags: map[string]string{"Owner": "alice"}, Attr: map[string]any{"state": "running"}}},
			},
			"dep-b": {
				Summary:   shared.DeploymentSummary{ID: "dep-b", Provider: "aws", Region: "us-east-1", Owner: "bob", CreatedAt: time.Unix(20, 0), State: "stopped", Resources: 1},
				Resources: []shared.ResourceMeta{{Ref: shared.ResourceRef{Type: "ec2-instance", ID: "i-b", ARN: "arn:b"}, Tags: map[string]string{"Owner": "bob"}, Attr: map[string]any{"state": "stopped"}}},
			},
		},
	}

	cleanupOpts.Providers = []string{"aws"}
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector { return collector }
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Given two deployment ids in a single show invocation
	// When the command runs
	err := cleanupShowCmd.RunE(cmd, []string{"dep-a", "dep-b"})

	// Then both deployments are rendered from one shared lookup pass
	if err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}
	rendered := stdout.String()
	for _, expected := range []string{"Scope:", "deployment=dep-a", "deployment=dep-b", "Summary: deployment=dep-a", "Summary: deployment=dep-b"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output missing %q: %s", expected, rendered)
		}
	}
}

func TestCleanupRunContinuesAcrossMultipleDeploymentIDs(t *testing.T) {
	originalOpts := cleanupOpts
	originalRunOpts := cleanupRunOpts
	originalAWSCollectorFactory := awsCollectorFactory
	originalExoscaleCollectorFactory := exoscaleCollectorFactory
	originalAWSCallerIdentityARNLookup := awsCallerIdentityARNLookup
	t.Cleanup(func() {
		cleanupOpts = originalOpts
		cleanupRunOpts = originalRunOpts
		awsCollectorFactory = originalAWSCollectorFactory
		exoscaleCollectorFactory = originalExoscaleCollectorFactory
		awsCallerIdentityARNLookup = originalAWSCallerIdentityARNLookup
	})

	collector := &scenarioProviderCollector{
		name:        "aws",
		accountInfo: "123456789012",
		summaries: []shared.DeploymentSummary{
			{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", Resources: 1},
			{ID: "dep-b", Provider: "aws", Region: "us-east-1", Owner: "bob", Resources: 1},
		},
		detailsByDeployment: map[string]*shared.DeploymentDetails{
			"dep-a": {Summary: shared.DeploymentSummary{ID: "dep-a", Provider: "aws", Region: "us-east-1", Owner: "alice", Resources: 1}},
			"dep-b": {Summary: shared.DeploymentSummary{ID: "dep-b", Provider: "aws", Region: "us-east-1", Owner: "bob", Resources: 1}},
		},
		resultsByDeployment: map[string][]shared.Result{
			"dep-a": {{Action: shared.Action{Ref: shared.ResourceRef{Type: "ec2-instance", ID: "i-a", ARN: "dep-a"}, Op: "delete", Reason: "ordered"}, Status: "planned"}},
			"dep-b": {{Action: shared.Action{Ref: shared.ResourceRef{Type: "ec2-instance", ID: "i-b", ARN: "dep-b"}, Op: "delete", Reason: "ordered"}, Status: "planned"}},
		},
	}

	cleanupOpts.Providers = []string{"aws"}
	awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector { return collector }
	exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
		return stubProviderCollector{name: "exoscale", accountInfo: "org-id"}
	}
	awsCallerIdentityARNLookup = func(context.Context, string) (string, bool) {
		return "arn:aws:iam::123456789012:role/example", true
	}

	var stdout bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Given multiple deployment ids including one missing id
	// When the cleanup run executes in dry-run mode
	err := cleanupRunCmd.RunE(cmd, []string{"dep-a", "missing", "dep-b"})

	// Then successful deployments are still processed and the command reports a batch failure
	if err == nil {
		t.Fatal("RunE returned nil, want batch failure")
	}
	if err.Error() != "1 deployment requests failed" {
		t.Fatalf("error = %q, want batch failure summary", err)
	}
	if len(collector.planCalls) != 2 {
		t.Fatalf("planCalls = %v, want 2 successful deployments", collector.planCalls)
	}
	rendered := stdout.String()
	for _, expected := range []string{"Cleanup DRY-RUN Deployment dep-a", "Cleanup DRY-RUN Deployment dep-b", "Requested: deployment=missing", "Error: deployment missing not found in searched providers"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output missing %q: %s", expected, rendered)
		}
	}
}
