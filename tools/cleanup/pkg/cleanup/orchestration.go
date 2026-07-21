// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package cleanup

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	ProviderStatusSearched    = "searched"
	ProviderStatusSkipped     = "skipped"
	ProviderStatusUnavailable = "unavailable"
	ProviderStatusError       = "error"

	ProviderReasonNotSelected = "not selected"
)

type Scope struct {
	Providers []ScopeProvider `json:"providers"`
}

type ScopeProvider struct {
	Provider    string `json:"provider"`
	Location    string `json:"location"`
	Owner       string `json:"owner"`
	OwnerSource string `json:"ownerSource,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
}

type OwnerResolution struct {
	Filter  string
	Display string
	Source  string
	Err     error
}

type ProviderSpec struct {
	Name           string
	Locations      func() []string
	ResolveOwner   func(context.Context, string) OwnerResolution
	BuildCollector func(string, string, bool) ProviderCollector
}

type ScopedCollector struct {
	Collector ProviderCollector
	Scope     *ScopeProvider
}

type Plan struct {
	Scope      Scope
	Collectors []ScopedCollector
}

type LookupMatch struct {
	Collector ProviderCollector
	Resolved  ResolvedDeployment
}

type ResolvedDeployment struct {
	Deployment string `json:"deployment"`
	Provider   string `json:"provider"`
	Location   string `json:"location"`
}

type LookupIndex struct {
	Matches      map[string][]LookupMatch
	SuccessCount int
}

func BuildPlan(ctx context.Context, specs []ProviderSpec, selectedProviders []string, legacy bool) Plan {
	plan := Plan{}
	for _, spec := range specs {
		selected := shouldUseProvider(spec.Name, selectedProviders)
		for _, location := range spec.Locations() {
			owner := spec.ResolveOwner(ctx, location)
			entry := ScopeProvider{
				Provider:    spec.Name,
				Location:    location,
				Owner:       owner.Display,
				OwnerSource: owner.Source,
				Status:      ProviderStatusSkipped,
				Reason:      ProviderReasonNotSelected,
			}

			plan.Scope.Providers = append(plan.Scope.Providers, entry)
			entryRef := &plan.Scope.Providers[len(plan.Scope.Providers)-1]
			if !selected {
				continue
			}

			if owner.Err != nil {
				entryRef.Status = ProviderStatusUnavailable
				entryRef.Reason = owner.Err.Error()
				continue
			}

			collector := spec.BuildCollector(location, owner.Filter, legacy)
			if _, err := collector.GetAccountInfo(ctx); err != nil {
				entryRef.Status = ProviderStatusUnavailable
				entryRef.Reason = err.Error()
				continue
			}

			entryRef.Status = ProviderStatusSearched
			entryRef.Reason = ""
			plan.Collectors = append(plan.Collectors, ScopedCollector{
				Collector: collector,
				Scope:     entryRef,
			})
		}
	}

	return plan
}

func CollectLookupIndex(ctx context.Context, plan *Plan) LookupIndex {
	lookupIndex := LookupIndex{
		Matches: make(map[string][]LookupMatch),
	}

	for _, scopedCollector := range plan.Collectors {
		summaries, err := scopedCollector.Collector.CollectDeploymentSummaries(ctx)
		if err != nil {
			scopedCollector.Scope.Status = ProviderStatusError
			scopedCollector.Scope.Reason = err.Error()
			continue
		}

		lookupIndex.SuccessCount++
		scopedCollector.Scope.Status = ProviderStatusSearched
		scopedCollector.Scope.Reason = ""
		for _, summary := range summaries {
			lookupIndex.Matches[summary.ID] = append(lookupIndex.Matches[summary.ID], LookupMatch{
				Collector: scopedCollector.Collector,
				Resolved: ResolvedDeployment{
					Deployment: summary.ID,
					Provider:   summary.Provider,
					Location:   summary.Region,
				},
			})
		}
	}

	return lookupIndex
}

func LookupDeploymentInPlan(ctx context.Context, plan *Plan, deploymentID string) ([]LookupMatch, int) {
	lookupIndex := CollectLookupIndex(ctx, plan)

	return lookupIndex.Matches[deploymentID], lookupIndex.SuccessCount
}

func LookupMatchSummary(matches []LookupMatch) string {
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, fmt.Sprintf("%s/%s", match.Resolved.Provider, match.Resolved.Location))
	}

	return strings.Join(parts, ", ")
}

func ResourceTypeFilter(typeNames []string) []ResourceType {
	if len(typeNames) == 0 {
		return nil
	}

	typeFilter := make([]ResourceType, 0, len(typeNames))
	for _, typeName := range typeNames {
		typeFilter = append(typeFilter, ResourceType(typeName))
	}

	return typeFilter
}

func FilterDeploymentDetailsByTypes(details *DeploymentDetails, typeFilter []ResourceType) *DeploymentDetails {
	if len(typeFilter) == 0 {
		return details
	}

	allowedTypes := make(map[ResourceType]struct{}, len(typeFilter))
	for _, resourceType := range typeFilter {
		allowedTypes[resourceType] = struct{}{}
	}

	filteredResources := make([]ResourceMeta, 0, len(details.Resources))
	for _, resource := range details.Resources {
		if _, ok := allowedTypes[resource.Ref.Type]; ok {
			filteredResources = append(filteredResources, resource)
		}
	}

	filteredDetails := *details
	filteredDetails.Resources = filteredResources
	filteredDetails.Summary = details.Summary
	filteredDetails.Summary.Resources = len(filteredResources)

	return &filteredDetails
}

func ResolvedLocations(explicit, defaults []string) []string {
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

func shouldUseProvider(providerName string, selectedProviders []string) bool {
	if len(selectedProviders) == 0 {
		return true
	}

	for _, name := range selectedProviders {
		if name == providerName {
			return true
		}
	}
	return false
}
