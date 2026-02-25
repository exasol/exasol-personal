// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/spf13/cobra"
)

var cleanupOpts = struct {
	Region  string
	Verbose bool
}{}

// Register persistent flags on the root command since we expose top-level
// subcommands (discover, show, run) without an intermediate "cleanup" group.
func registerRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().
		StringVar(&cleanupOpts.Region, "region", "",
			"AWS region containing the deployment resources")
	cmd.PersistentFlags().
		BoolVar(&cleanupOpts.Verbose, "verbose", false,
			"Enable verbose (debug) logging")
}

// nolint: gochecknoinits
func init() {
	registerRootFlags(rootCmd)
}
