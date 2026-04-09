// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log/slog"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/aws"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/exoscale"
	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/spf13/cobra"
)

const cleanupProvidersShort = "List available providers and connection status"

var cleanupProvidersCmd = &cobra.Command{
	Use:    "providers",
	Short:  cleanupProvidersShort,
	Args:   cobra.NoArgs,
	PreRun: func(_ *cobra.Command, _ []string) { configureLogger() },
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		var collectors []shared.ProviderCollector

		// AWS provider
		if shouldUseProvider(aws.ProviderName) {
			awsRegion := cleanupOpts.AWSRegion
			if awsRegion == "" {
				awsRegion = "us-east-1" // Default region for availability check
			}
			collectors = append(collectors, aws.NewCollector(awsRegion, "", false))
		}

		// Exoscale provider
		if shouldUseProvider(exoscale.ProviderName) {
			exoscaleZone := cleanupOpts.ExoscaleZone
			if exoscaleZone == "" {
				exoscaleZone = "ch-gva-2" // Already has default, but be explicit
			}
			collectors = append(collectors, exoscale.NewCollector(exoscaleZone, "", false))
		}

		for _, collector := range collectors {
			providerName := collector.Name()
			// Capitalize and pad provider name for alignment
			displayName := providerName
			if displayName == "aws" {
				displayName = "AWS"
			} else if displayName == "exoscale" {
				displayName = "Exoscale"
			}

			// Pad to 11 characters for alignment
			padded := fmt.Sprintf("%-11s", displayName)

			if !collector.IsAvailable(cmd.Context()) {
				fmt.Printf("%s Disconnected\n", padded)
				slog.Debug("provider not available", "provider", providerName)
				continue
			}

			accountInfo, err := collector.GetAccountInfo(cmd.Context())
			if err != nil {
				fmt.Printf("%s Disconnected\n", padded)
				slog.Debug("failed to get account info",
					"provider", providerName,
					"error", err)
				continue
			}

			fmt.Printf("%s Connected    %s\n", padded, accountInfo)
		}

		return nil
	},
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupProvidersCmd)
}
