// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/exasol/exasol-personal/tools/cleanup/internal/aws"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/exoscale"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/stackit"
	"github.com/spf13/cobra"
)

var cleanupOpts = struct {
	AWSRegion        string
	ExoscaleZone     string
	STACKITRegion    string
	STACKITProjectId string
	Verbose          bool
	AWS              bool
	Exoscale         bool
	STACKIT          bool
}{}

// getSelectedProviders returns a list of provider names that should be used.
// If no provider flags are set, returns all available providers.
// If any provider flag is set, returns only those selected.
func getSelectedProviders() []string {
	var selected []string

	if cleanupOpts.AWS {
		selected = append(selected, aws.ProviderName)
	}
	if cleanupOpts.Exoscale {
		selected = append(selected, exoscale.ProviderName)
	}
	if cleanupOpts.STACKIT {
		selected = append(selected, stackit.ProviderName)
	}

	// If none selected, use all available providers
	if len(selected) == 0 {
		return []string{aws.ProviderName, exoscale.ProviderName, stackit.ProviderName}
	}

	return selected
}

// shouldUseProvider checks if a provider should be used based on the selection
func shouldUseProvider(providerName string) bool {
	selected := getSelectedProviders()
	for _, name := range selected {
		if name == providerName {
			return true
		}
	}
	return false
}

// Register persistent flags on the root command since we expose top-level
// subcommands (discover, show, run) without an intermediate "cleanup" group.
func registerRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().
		StringVar(&cleanupOpts.AWSRegion, "aws-region", "",
			"AWS region containing the deployment resources")
	cmd.PersistentFlags().
		StringVar(&cleanupOpts.ExoscaleZone, "exoscale-zone", "ch-gva-2",
			"Exoscale zone containing the deployment resources (default: ch-gva-2)")
	cmd.PersistentFlags().
		StringVar(&cleanupOpts.STACKITRegion, "stackit-region", "eu01",
			"STACKIT region containing the deployment resources")
	cmd.PersistentFlags().
		StringVar(&cleanupOpts.STACKITProjectId, "stackit-project-id", "",
			"STACKIT project containing the deployment resources")
	cmd.PersistentFlags().
		BoolVar(&cleanupOpts.Verbose, "verbose", false,
			"Enable verbose (debug) logging")
	cmd.PersistentFlags().
		BoolVar(&cleanupOpts.AWS, "aws", false,
			"Use AWS provider")
	cmd.PersistentFlags().
		BoolVar(&cleanupOpts.Exoscale, "exoscale", false,
			"Use Exoscale provider")
	cmd.PersistentFlags().
		BoolVar(&cleanupOpts.STACKIT, "stackit", false,
			"Use STACKIT provider")

	cmd.MarkFlagsRequiredTogether("stackit", "stackit-project-id")
}

// nolint: gochecknoinits
func init() {
	registerRootFlags(rootCmd)
}
