// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import "github.com/spf13/cobra"

const shellRootCmdShortDesc = "Shell access to deployment host and container"

const shellRootCmdLongDesc = shellRootCmdShortDesc + `

Includes subcommands for host OS shell and COS container shell access.
`

var shellRootCmd = &cobra.Command{
	Use:   "shell",
	Short: shellRootCmdShortDesc,
	Long:  shellRootCmdLongDesc,
	Args:  cobra.NoArgs,
}

// nolint: gochecknoinits
func init() {
	rootCmd.AddCommand(shellRootCmd)
}
