// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/aws"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/azure"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/exoscale"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/stackit"
)

const (
	providerStatusSearched    = "searched"
	providerStatusSkipped     = "skipped"
	providerStatusUnavailable = "unavailable"
	providerStatusError       = "error"

	providerReasonNotSelected     = "not selected"
	providerReasonCallerLookupErr = "failed to resolve caller identity"
)

var awsCollectorFactory = func(region, ownerFilter string, legacy bool) shared.ProviderCollector {
	return aws.NewCollector(region, ownerFilter, legacy)
}

var exoscaleCollectorFactory = func(zone, ownerFilter string, legacy bool) shared.ProviderCollector {
	return exoscale.NewCollector(zone, ownerFilter, legacy)
}

var stackitCollectorFactory = func(projectID, region, ownerFilter string) shared.ProviderCollector {
	return stackit.NewCollector(projectID, region, ownerFilter)
}

var azureCollectorFactory = func(subscriptionID, location, ownerFilter string, legacy bool) shared.ProviderCollector {
	return azure.NewCollector(subscriptionID, location, ownerFilter, legacy)
}

var awsCallerIdentityARNLookup = func(ctx context.Context, region string) (string, bool) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", false
	}

	stsClient := sts.NewFromConfig(cfg)
	idOut, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil || idOut.Arn == nil || *idOut.Arn == "" {
		return "", false
	}

	return *idOut.Arn, true
}

type cleanupCommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type cleanupDiscoverySummary struct {
	Count int `json:"count"`
}

type cleanupExecutionSummary struct {
	Actions int `json:"actions"`
}

type cleanupExecutionInfo struct {
	Mode  string   `json:"mode"`
	Types []string `json:"types,omitempty"`
}

type cleanupResolved struct {
	Deployment string `json:"deployment"`
	Provider   string `json:"provider"`
	Location   string `json:"location"`
}

type cleanupDiscoverJSONOutput struct {
	Scope   cleanupScope               `json:"scope"`
	Data    []shared.DeploymentSummary `json:"data,omitempty"`
	Summary *cleanupDiscoverySummary   `json:"summary,omitempty"`
	Error   *cleanupCommandError       `json:"error,omitempty"`
}

type cleanupShowJSONOutput struct {
	Scope       cleanupScope                      `json:"scope"`
	Deployments []cleanupShowDeploymentJSONOutput `json:"deployments"`
	Error       *cleanupCommandError              `json:"error,omitempty"`
}

type cleanupRunJSONOutput struct {
	Scope       cleanupScope                     `json:"scope"`
	Execution   cleanupExecutionInfo             `json:"execution"`
	Deployments []cleanupRunDeploymentJSONOutput `json:"deployments"`
	Error       *cleanupCommandError             `json:"error,omitempty"`
}

type cleanupShowDeploymentJSONOutput struct {
	Requested string                    `json:"requested"`
	Resolved  *cleanupResolved          `json:"resolved,omitempty"`
	Details   *shared.DeploymentDetails `json:"details,omitempty"`
	Error     *cleanupCommandError      `json:"error,omitempty"`
}

type cleanupRunDeploymentJSONOutput struct {
	Requested string                   `json:"requested"`
	Resolved  *cleanupResolved         `json:"resolved,omitempty"`
	Results   []shared.Result          `json:"results,omitempty"`
	Summary   *cleanupExecutionSummary `json:"summary,omitempty"`
	Error     *cleanupCommandError     `json:"error,omitempty"`
}

type cleanupScope struct {
	Providers []cleanupScopeProvider `json:"providers"`
}

type cleanupScopeProvider struct {
	Provider    string `json:"provider"`
	Location    string `json:"location"`
	Owner       string `json:"owner"`
	OwnerSource string `json:"ownerSource,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
}

type cleanupOwnerResolution struct {
	Filter  string
	Display string
	Source  string
	Err     error
}

type cleanupProviderSpec struct {
	Name           string
	Locations      func() []string
	ResolveOwner   func(context.Context, string) cleanupOwnerResolution
	BuildCollector func(string, string, bool) shared.ProviderCollector
}

type cleanupScopedCollector struct {
	Collector shared.ProviderCollector
	Scope     *cleanupScopeProvider
}

type cleanupPlan struct {
	Scope      cleanupScope
	Collectors []cleanupScopedCollector
}

type cleanupLookupMatch struct {
	Collector shared.ProviderCollector
	Resolved  cleanupResolved
}

type cleanupLookupIndex struct {
	Matches      map[string][]cleanupLookupMatch
	SuccessCount int
}

func buildCleanupPlan(ctx context.Context, legacy bool) cleanupPlan {
	plan := cleanupPlan{}
	for _, spec := range cleanupProviderSpecs() {
		selected := shouldUseProvider(spec.Name)
		for _, location := range spec.Locations() {
			owner := spec.ResolveOwner(ctx, location)
			entry := cleanupScopeProvider{
				Provider:    spec.Name,
				Location:    location,
				Owner:       owner.Display,
				OwnerSource: owner.Source,
				Status:      providerStatusSkipped,
				Reason:      providerReasonNotSelected,
			}

			plan.Scope.Providers = append(plan.Scope.Providers, entry)
			entryRef := &plan.Scope.Providers[len(plan.Scope.Providers)-1]
			if !selected {
				continue
			}

			if owner.Err != nil {
				entryRef.Status = providerStatusUnavailable
				entryRef.Reason = owner.Err.Error()
				continue
			}

			collector := spec.BuildCollector(location, owner.Filter, legacy)
			if _, err := collector.GetAccountInfo(ctx); err != nil {
				entryRef.Status = providerStatusUnavailable
				entryRef.Reason = err.Error()
				continue
			}

			entryRef.Status = providerStatusSearched
			entryRef.Reason = ""
			plan.Collectors = append(plan.Collectors, cleanupScopedCollector{
				Collector: collector,
				Scope:     entryRef,
			})
		}
	}

	return plan
}

func lookupDeploymentInPlan(
	ctx context.Context,
	plan *cleanupPlan,
	deploymentID string,
) ([]cleanupLookupMatch, int) {
	lookupIndex := collectLookupIndex(ctx, plan)

	return lookupIndex.Matches[deploymentID], lookupIndex.SuccessCount
}

func collectLookupIndex(ctx context.Context, plan *cleanupPlan) cleanupLookupIndex {
	lookupIndex := cleanupLookupIndex{
		Matches: make(map[string][]cleanupLookupMatch),
	}

	for _, scopedCollector := range plan.Collectors {
		summaries, err := scopedCollector.Collector.CollectDeploymentSummaries(ctx)
		if err != nil {
			scopedCollector.Scope.Status = providerStatusError
			scopedCollector.Scope.Reason = err.Error()
			continue
		}

		lookupIndex.SuccessCount++
		scopedCollector.Scope.Status = providerStatusSearched
		scopedCollector.Scope.Reason = ""
		for _, summary := range summaries {
			lookupIndex.Matches[summary.ID] = append(lookupIndex.Matches[summary.ID], cleanupLookupMatch{
				Collector: scopedCollector.Collector,
				Resolved: cleanupResolved{
					Deployment: summary.ID,
					Provider:   summary.Provider,
					Location:   summary.Region,
				},
			})
		}
	}

	return lookupIndex
}

func lookupMatchSummary(matches []cleanupLookupMatch) string {
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, fmt.Sprintf("%s/%s", match.Resolved.Provider, match.Resolved.Location))
	}

	return strings.Join(parts, ", ")
}

func cleanupProviderSpecs() []cleanupProviderSpec {
	return []cleanupProviderSpec{
		{
			Name:         aws.ProviderName,
			Locations:    resolvedAWSRegions,
			ResolveOwner: resolveAWSOwner,
			BuildCollector: func(location, ownerFilter string, legacy bool) shared.ProviderCollector {
				return awsCollectorFactory(location, ownerFilter, legacy)
			},
		},
		{
			Name:         exoscale.ProviderName,
			Locations:    resolvedExoscaleZones,
			ResolveOwner: resolveExoscaleOwner,
			BuildCollector: func(location, ownerFilter string, legacy bool) shared.ProviderCollector {
				return exoscaleCollectorFactory(location, ownerFilter, legacy)
			},
		},
		{
			Name:         stackit.ProviderName,
			Locations:    resolvedStackitRegions,
			ResolveOwner: resolveStackitOwner,
			BuildCollector: func(location, ownerFilter string, _ bool) shared.ProviderCollector {
				return stackitCollectorFactory(cleanupOpts.StackitProjectID, location, ownerFilter)
			},
		},
		{
			Name:         azure.ProviderName,
			Locations:    resolvedAzureLocations,
			ResolveOwner: resolveAzureOwner,
			BuildCollector: func(location, ownerFilter string, legacy bool) shared.ProviderCollector {
				return azureCollectorFactory(cleanupOpts.AzureSubscriptionID, location, ownerFilter, legacy)
			},
		},
	}
}

func resolveAWSOwner(ctx context.Context, region string) cleanupOwnerResolution {
	if cleanupOpts.OwnerFilter != "" {
		return cleanupOwnerResolution{Filter: cleanupOpts.OwnerFilter, Display: cleanupOpts.OwnerFilter, Source: "explicit"}
	}

	filter, ok := awsCallerIdentityARNLookup(ctx, region)
	if !ok {
		return cleanupOwnerResolution{
			Display: "(caller)",
			Source:  "default",
			Err:     fmt.Errorf(providerReasonCallerLookupErr),
		}
	}

	return cleanupOwnerResolution{Filter: filter, Display: "(caller)", Source: "default"}
}

func resolveExoscaleOwner(_ context.Context, _ string) cleanupOwnerResolution {
	if cleanupOpts.OwnerFilter != "" {
		return cleanupOwnerResolution{Filter: cleanupOpts.OwnerFilter, Display: cleanupOpts.OwnerFilter, Source: "explicit"}
	}

	return cleanupOwnerResolution{Display: "*", Source: "default"}
}

func resolveStackitOwner(_ context.Context, _ string) cleanupOwnerResolution {
	if cleanupOpts.StackitProjectID == "" {
		return cleanupOwnerResolution{
			Display: "*",
			Source:  "default",
			Err:     fmt.Errorf("STACKIT project id is required; pass --stackit-project-id"),
		}
	}

	if cleanupOpts.OwnerFilter != "" {
		return cleanupOwnerResolution{Filter: cleanupOpts.OwnerFilter, Display: cleanupOpts.OwnerFilter, Source: "explicit"}
	}

	return cleanupOwnerResolution{Display: "*", Source: "default"}
}

func resolveAzureOwner(_ context.Context, _ string) cleanupOwnerResolution {
	if cleanupOpts.AzureSubscriptionID == "" {
		return cleanupOwnerResolution{
			Display: "*",
			Source:  "default",
			Err:     fmt.Errorf("Azure subscription id is required; pass --azure-subscription-id"),
		}
	}

	if cleanupOpts.OwnerFilter != "" {
		return cleanupOwnerResolution{Filter: cleanupOpts.OwnerFilter, Display: cleanupOpts.OwnerFilter, Source: "explicit"}
	}

	return cleanupOwnerResolution{Display: "*", Source: "default"}
}

func resolvedAWSRegions() []string {
	return resolvedLocations(cleanupOpts.AWSRegions, []string{"us-east-1"})
}

func resolvedExoscaleZones() []string {
	return resolvedLocations(cleanupOpts.ExoscaleZones, []string{"ch-gva-2"})
}

func resolvedStackitRegions() []string {
	return resolvedLocations(cleanupOpts.StackitRegions, []string{"eu01"})
}

func resolvedAzureLocations() []string {
	return resolvedLocations(cleanupOpts.AzureLocations, []string{azure.DefaultLocation})
}

func resolvedLocations(explicit []string, defaults []string) []string {
	if len(explicit) == 0 {
		return append([]string(nil), defaults...)
	}

	locations := make([]string, 0, len(explicit))
	for _, location := range explicit {
		if location == "" || slices.Contains(locations, location) {
			continue
		}
		locations = append(locations, location)
	}

	if len(locations) == 0 {
		return append([]string(nil), defaults...)
	}

	return locations
}

func renderCleanupScope(writer io.Writer, scope cleanupScope) {
	if len(scope.Providers) == 0 {
		return
	}

	if _, err := fmt.Fprintln(writer, "Scope:"); err != nil {
		return
	}

	rows := make([][]string, 0, len(scope.Providers))
	for _, provider := range scope.Providers {
		reason := provider.Reason
		if reason == "" {
			reason = "-"
		}
		rows = append(rows, []string{
			provider.Provider,
			provider.Location,
			provider.Owner,
			provider.Status,
			reason,
		})
	}

	shared.RenderTable(
		writer,
		[]string{"provider", "location", "owner", "status", "reason"},
		[]int{12, 14, 28, 12, 40},
		rows,
	)
	_, _ = fmt.Fprintln(writer)
}

func renderResolved(writer io.Writer, resolved cleanupResolved) {
	_, _ = fmt.Fprintf(
		writer,
		"Resolved: deployment=%s provider=%s location=%s\n\n",
		resolved.Deployment,
		resolved.Provider,
		resolved.Location,
	)
}

func renderRequestedDeployment(writer io.Writer, deploymentID string) {
	_, _ = fmt.Fprintf(writer, "Requested: deployment=%s\n\n", deploymentID)
}

func renderTypesFilter(writer io.Writer, types []string) {
	typeDisplay := "(all)"
	if len(types) > 0 {
		typeDisplay = strings.Join(types, ",")
	}
	_, _ = fmt.Fprintf(writer, "Filters: types=%s\n\n", typeDisplay)
}

func renderExecutionInfo(writer io.Writer, execution cleanupExecutionInfo) {
	typeDisplay := "(all)"
	if len(execution.Types) > 0 {
		typeDisplay = strings.Join(execution.Types, ",")
	}
	_, _ = fmt.Fprintf(writer, "Execution: mode=%s types=%s\n\n", execution.Mode, typeDisplay)
}

func encodeJSONOutput(writer io.Writer, payload any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(payload)
}

func commandError(code string, err error) *cleanupCommandError {
	if err == nil {
		return nil
	}

	return &cleanupCommandError{Code: code, Message: err.Error()}
}
