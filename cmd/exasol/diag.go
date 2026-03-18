// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/spf13/cobra"
)

const diagCmdShortDesc = "Diagnostic tools for an active deployment"

const diagCmdLongDesc = diagCmdShortDesc + `

Includes subcommands for deployment configuration inspection and recovery tooling.
`

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: diagCmdShortDesc,
	Long:  diagCmdLongDesc,
	Args:  cobra.NoArgs,
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(diagCmd)
}
