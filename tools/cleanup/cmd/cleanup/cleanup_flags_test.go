// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterCommonFlagsScopesOptionalFlags(t *testing.T) {
	t.Parallel()

	commandWithOwnerAndJSON := &cobra.Command{Use: "discover"}
	registerCommonFlags(commandWithOwnerAndJSON, cleanupFlagOptions{includeOwner: true, includeJSON: true})

	providersCommand := &cobra.Command{Use: "providers"}
	registerCommonFlags(providersCommand, cleanupFlagOptions{})

	// Given command-specific flag registration
	// When optional flags are enabled only where needed
	// Then owner/json are available on the intended command but absent on providers
	if commandWithOwnerAndJSON.Flags().Lookup("owner") == nil {
		t.Fatal("discover flags are missing owner")
	}
	if commandWithOwnerAndJSON.Flags().Lookup("json") == nil {
		t.Fatal("discover flags are missing json")
	}
	if providersCommand.Flags().Lookup("owner") != nil {
		t.Fatal("providers unexpectedly exposes owner")
	}
	if providersCommand.Flags().Lookup("json") != nil {
		t.Fatal("providers unexpectedly exposes json")
	}
	if providersCommand.Flags().Lookup("provider") == nil {
		t.Fatal("providers is missing shared provider-selection flag")
	}
}

func TestCleanupHelpGroupsProviderFlags(t *testing.T) {
	command := &cobra.Command{Use: "discover"}
	registerCommonFlags(command, cleanupFlagOptions{includeOwner: true, includeJSON: true})

	var output bytes.Buffer
	renderCleanupFlagGroups(&output, command)
	rendered := output.String()

	// Given grouped flag rendering
	// When a command has generic and provider-specific options
	// Then provider flags are separated into provider option groups
	options := sectionBetween(t, rendered, cleanupFlagGroupGeneric+":", cleanupFlagGroupAWS+":")
	awsOptions := sectionBetween(t, rendered, cleanupFlagGroupAWS+":", cleanupFlagGroupExoscale+":")
	exoscaleOptions := sectionBetween(t, rendered, cleanupFlagGroupExoscale+":", cleanupFlagGroupStackit+":")
	stackitOptions := sectionAfter(t, rendered, cleanupFlagGroupStackit+":")

	for _, expected := range []string{"--help", "--json", "--owner", "--provider", "--verbose"} {
		if !strings.Contains(options, expected) {
			t.Fatalf("generic options missing %q:\n%s", expected, rendered)
		}
	}
	for _, unexpected := range []string{"--aws-region", "--exoscale-zone", "--stackit-project-id", "--stackit-region"} {
		if strings.Contains(options, unexpected) {
			t.Fatalf("generic options unexpectedly contain provider flag %q:\n%s", unexpected, rendered)
		}
	}
	if !strings.Contains(awsOptions, "--aws-region") {
		t.Fatalf("AWS options missing --aws-region:\n%s", rendered)
	}
	if !strings.Contains(exoscaleOptions, "--exoscale-zone") {
		t.Fatalf("Exoscale options missing --exoscale-zone:\n%s", rendered)
	}
	for _, expected := range []string{"--stackit-project-id", "--stackit-region"} {
		if !strings.Contains(stackitOptions, expected) {
			t.Fatalf("STACKIT options missing %q:\n%s", expected, rendered)
		}
	}
}

func sectionBetween(t *testing.T, text, start, end string) string {
	t.Helper()
	startIndex := strings.Index(text, start)
	if startIndex < 0 {
		t.Fatalf("missing section %q in:\n%s", start, text)
	}
	sectionStart := startIndex + len(start)
	endIndex := strings.Index(text[sectionStart:], end)
	if endIndex < 0 {
		t.Fatalf("missing section %q after %q in:\n%s", end, start, text)
	}

	return text[sectionStart : sectionStart+endIndex]
}

func sectionAfter(t *testing.T, text, start string) string {
	t.Helper()
	startIndex := strings.Index(text, start)
	if startIndex < 0 {
		t.Fatalf("missing section %q in:\n%s", start, text)
	}

	return text[startIndex+len(start):]
}

func TestGetSelectedProvidersDefaultsToAll(t *testing.T) {
	t.Parallel()

	originalOpts := cleanupOpts
	t.Cleanup(func() { cleanupOpts = originalOpts })

	cleanupOpts.Providers = nil

	selected := getSelectedProviders()
	if len(selected) != 3 {
		t.Fatalf("selected = %v, want 3 providers", selected)
	}
	if selected[0] != "aws" || selected[1] != "exoscale" || selected[2] != "stackit" {
		t.Fatalf("selected = %v, want [aws exoscale stackit]", selected)
	}
}

func TestGetSelectedProvidersUsesExplicitListWithoutDuplicates(t *testing.T) {
	t.Parallel()

	originalOpts := cleanupOpts
	t.Cleanup(func() { cleanupOpts = originalOpts })

	cleanupOpts.Providers = []string{"exoscale", "aws", "exoscale"}

	selected := getSelectedProviders()
	if len(selected) != 2 {
		t.Fatalf("selected = %v, want 2 providers", selected)
	}
	if selected[0] != "exoscale" || selected[1] != "aws" {
		t.Fatalf("selected = %v, want [exoscale aws]", selected)
	}
}

func TestResolvedLocationsDefaultsAndDeduplicates(t *testing.T) {
	t.Parallel()

	originalOpts := cleanupOpts
	t.Cleanup(func() { cleanupOpts = originalOpts })

	if got := resolvedAWSRegions(); len(got) != 1 || got[0] != "us-east-1" {
		t.Fatalf("resolvedAWSRegions = %v, want [us-east-1]", got)
	}

	cleanupOpts.AWSRegions = []string{"eu-central-1", "eu-central-1", "", "us-east-1"}
	if got := resolvedAWSRegions(); len(got) != 2 || got[0] != "eu-central-1" || got[1] != "us-east-1" {
		t.Fatalf("resolvedAWSRegions = %v, want [eu-central-1 us-east-1]", got)
	}

	if got := resolvedExoscaleZones(); len(got) != 1 || got[0] != "ch-gva-2" {
		t.Fatalf("resolvedExoscaleZones = %v, want [ch-gva-2]", got)
	}

	if got := resolvedStackitRegions(); len(got) != 1 || got[0] != "eu01" {
		t.Fatalf("resolvedStackitRegions = %v, want [eu01]", got)
	}
}
