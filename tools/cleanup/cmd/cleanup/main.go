// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const shortDescription = "Exasol Personal cleanup tool"

const description = shortDescription + `
Specific providers can be targeted using the --aws and --exoscale flags. If no provider flags are set, all providers will be used.
`

var rootCmd = &cobra.Command{
	Use:   "exasol-cleanup",
	Short: shortDescription,
	Long:  description,
}


func configureLogger() {
	level := slog.LevelInfo
	if cleanupOpts.Verbose {
		level = slog.LevelDebug
	}
	
	var handler slog.Handler
	if term.IsTerminal(int(os.Stderr.Fd())) {
		// When attached to a terminal, use human-friendly colored logs
		handler = tint.NewHandler(os.Stderr, &tint.Options{
			Level:      level,
			TimeFormat: time.DateTime,
		})
	} else {
		// When redirected/piped, use JSON for machine parsing
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	}
	
	slog.SetDefault(slog.New(handler))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command execution failed", "error", err)
		os.Exit(1)
	}
}
