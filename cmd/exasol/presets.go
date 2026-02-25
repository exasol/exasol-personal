// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/spf13/cobra"
)

const presetsCmdShortDesc = "Manage embedded presets"

const presetsCmdLongDesc = presetsCmdShortDesc + `

Presets are pre-defined configurations used by commands like "init" and "install".

Use "exasol presets list" to see what presets are available.
`

var presetsCmd = &cobra.Command{
	Use:   "presets",
	Short: presetsCmdShortDesc,
	Long:  presetsCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

// nolint: gochecknoinits
func init() {
	registerPresetsListCmd(presetsCmd)
	registerPresetsExportCmd(presetsCmd)
	rootCmd.AddCommand(presetsCmd)
}
