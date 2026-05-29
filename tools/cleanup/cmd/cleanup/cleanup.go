// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/aws"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/exoscale"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/stackit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	cleanupFlagGroupAnnotation = "exasol-cleanup/flag-group"
	cleanupFlagGroupGeneric    = "Options"
	cleanupFlagGroupAWS        = "AWS options"
	cleanupFlagGroupExoscale   = "Exoscale options"
	cleanupFlagGroupStackit    = "STACKIT options"
)

var cleanupFlagGroupOrder = []string{
	cleanupFlagGroupGeneric,
	cleanupFlagGroupAWS,
	cleanupFlagGroupExoscale,
	cleanupFlagGroupStackit,
}

type cleanupFlagOptions struct {
	includeOwner bool
	includeJSON  bool
}

var cleanupOpts = struct {
	AWSRegions       []string
	ExoscaleZones    []string
	StackitRegions   []string
	StackitProjectID string
	Providers        []string
	OwnerFilter      string
	JSON             bool
	Verbose          bool
}{}

func allProviders() []string {
	return []string{aws.ProviderName, exoscale.ProviderName, stackit.ProviderName}
}

// getSelectedProviders returns a list of provider names that should be used.
// If no provider list is set, returns all available providers.
// If any providers are set, returns only those selected.
func getSelectedProviders() []string {
	if len(cleanupOpts.Providers) == 0 {
		return allProviders()
	}

	selected := make([]string, 0, len(cleanupOpts.Providers))
	for _, provider := range cleanupOpts.Providers {
		if !slices.Contains(selected, provider) {
			selected = append(selected, provider)
		}
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

func registerCommonFlags(cmd *cobra.Command, options cleanupFlagOptions) {
	cmd.Flags().
		StringSliceVar(&cleanupOpts.AWSRegions, "aws-region", nil,
			"AWS regions containing deployment resources (repeat flag or use comma-separated values)")
	setFlagGroup(cmd, "aws-region", cleanupFlagGroupAWS)
	cmd.Flags().
		StringSliceVar(&cleanupOpts.ExoscaleZones, "exoscale-zone", nil,
			"Exoscale zones containing deployment resources (repeat flag or use comma-separated values; default: ch-gva-2)")
	setFlagGroup(cmd, "exoscale-zone", cleanupFlagGroupExoscale)
	cmd.Flags().
		StringSliceVar(&cleanupOpts.StackitRegions, "stackit-region", nil,
			"STACKIT regions containing deployment resources (repeat flag or use comma-separated values; default: eu01)")
	setFlagGroup(cmd, "stackit-region", cleanupFlagGroupStackit)
	cmd.Flags().
		StringVar(&cleanupOpts.StackitProjectID, "stackit-project-id", "",
			"STACKIT project containing deployment resources")
	setFlagGroup(cmd, "stackit-project-id", cleanupFlagGroupStackit)
	cmd.Flags().
		BoolVar(&cleanupOpts.Verbose, "verbose", false,
			"Enable verbose (debug) logging")
	cmd.Flags().
		StringSliceVar(&cleanupOpts.Providers, "provider", nil,
			fmt.Sprintf("Providers to use (default: %s)", strings.Join(allProviders(), ",")))

	if options.includeOwner {
		cmd.Flags().
			StringVar(&cleanupOpts.OwnerFilter, "owner", "",
				"Owner ARN/wildcard to filter (defaults AWS to caller; use * for any)")
	}

	if options.includeJSON {
		cmd.Flags().
			BoolVar(&cleanupOpts.JSON, "json", false,
				"Output JSON")
	}
}

func setFlagGroup(cmd *cobra.Command, flagName, group string) {
	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[cleanupFlagGroupAnnotation] = []string{group}
}

func cleanupHelpFunc(cmd *cobra.Command, _ []string) {
	writer := cmd.OutOrStdout()
	if cmd.Long != "" {
		_, _ = fmt.Fprintln(writer, strings.TrimSpace(cmd.Long))
		_, _ = fmt.Fprintln(writer)
	} else if cmd.Short != "" {
		_, _ = fmt.Fprintln(writer, cmd.Short)
		_, _ = fmt.Fprintln(writer)
	}

	if cmd.Runnable() || cmd.HasAvailableSubCommands() {
		_, _ = fmt.Fprintln(writer, "Usage:")
		_, _ = fmt.Fprintf(writer, "  %s\n\n", cmd.UseLine())
	}

	cmd.InitDefaultHelpCmd()
	if cmd.HasAvailableSubCommands() {
		_, _ = fmt.Fprintln(writer, "Available Commands:")
		for _, subcommand := range cmd.Commands() {
			if !subcommand.IsAvailableCommand() && subcommand.Name() != "help" {
				continue
			}
			_, _ = fmt.Fprintf(writer, "  %-12s %s\n", subcommand.Name(), subcommand.Short)
		}
		_, _ = fmt.Fprintln(writer)
	}

	renderCleanupFlagGroups(writer, cmd)

	if cmd.HasAvailableSubCommands() {
		_, _ = fmt.Fprintf(writer, "Use \"%s [command] --help\" for more information about a command.\n", cmd.CommandPath())
	}
}

func renderCleanupFlagGroups(writer io.Writer, cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	groups := map[string][]*pflag.Flag{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		group := flagGroup(flag)
		groups[group] = append(groups[group], flag)
	})

	renderedGroups := map[string]bool{}
	for _, group := range cleanupFlagGroupOrder {
		renderFlagGroup(writer, group, groups[group])
		renderedGroups[group] = true
	}
	for group, flags := range groups {
		if renderedGroups[group] {
			continue
		}
		renderFlagGroup(writer, group, flags)
	}
}

func flagGroup(flag *pflag.Flag) string {
	if flag.Annotations == nil {
		return cleanupFlagGroupGeneric
	}
	groups := flag.Annotations[cleanupFlagGroupAnnotation]
	if len(groups) == 0 || groups[0] == "" {
		return cleanupFlagGroupGeneric
	}

	return groups[0]
}

func renderFlagGroup(writer io.Writer, title string, flags []*pflag.Flag) {
	if len(flags) == 0 {
		return
	}

	flagSet := pflag.NewFlagSet(title, pflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	for _, flag := range flags {
		flagCopy := *flag
		flagSet.AddFlag(&flagCopy)
	}

	_, _ = fmt.Fprintf(writer, "%s:\n", title)
	_, _ = fmt.Fprint(writer, flagSet.FlagUsagesWrapped(100))
	_, _ = fmt.Fprintln(writer)
}
