// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"log/slog"

	"github.com/spf13/cobra"
)

const cleanupProvidersShort = "List available providers and connection status"

type providerStatus struct {
	Provider  string `json:"provider"`
	Location  string `json:"location"`
	Connected bool   `json:"connected"`
	Account   string `json:"account,omitempty"`
}

var cleanupProvidersCmd = &cobra.Command{
	Use:    "providers",
	Short:  cleanupProvidersShort,
	Args:   cobra.NoArgs,
	PreRun: func(_ *cobra.Command, _ []string) { configureLogger() },
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		statuses := make([]providerStatus, 0)
		for _, spec := range cleanupProviderSpecs() {
			if !shouldUseProvider(spec.Name) {
				continue
			}
			for _, location := range spec.Locations() {
				collector := spec.BuildCollector(location, "", false)
				status := providerStatus{Provider: spec.Name, Location: location, Connected: false}
				accountInfo, err := collector.GetAccountInfo(cmd.Context())
				if err != nil {
					statuses = append(statuses, status)
					slog.Debug("failed to get account info",
						"provider", spec.Name,
						"location", location,
						"error", err)
					continue
				}

				status.Connected = true
				status.Account = accountInfo
				statuses = append(statuses, status)
			}
		}

		if cleanupOpts.JSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")

			return enc.Encode(statuses)
		}

		rows := make([][]string, 0, len(statuses))
		for _, status := range statuses {
			connected := "disconnected"
			account := "-"
			if status.Connected {
				connected = "connected"
				account = status.Account
			}
			rows = append(rows, []string{status.Provider, status.Location, connected, account})
		}
		if len(rows) > 0 {
			renderTable(
				cmd.OutOrStdout(),
				[]string{"provider", "location", "status", "account"},
				[]int{12, 14, 12, 24},
				rows,
			)
		}

		return nil
	},
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(cleanupProvidersCmd)
	registerCommonFlags(cleanupProvidersCmd, cleanupFlagOptions{})
}
