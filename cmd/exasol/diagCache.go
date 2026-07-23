// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"

	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/spf13/cobra"
)

var diagCacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Inspect the runtime artifact cache",
	Long: `Inspect the runtime artifact cache.

This command reports cache state without removing artifacts, rewriting metadata,
or clearing cache locks.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		artifactCache, err := runtimeartifacts.NewDefaultCache()
		if err != nil {
			return err
		}

		return addRenderedTerminalOutput(func(writer io.Writer) error {
			return renderCacheDiagnosticsText(writer, artifactCache.Diagnose())
		})
	},
}

func renderCacheDiagnosticsText(
	writer io.Writer,
	report runtimeartifacts.DiagnosticReport,
) error {
	lines := []string{
		"Cache root: " + report.CacheRoot,
		"Config file: " + report.ConfigPath,
		cacheConfigStatusLine(report),
		"Index file: " + formatCachePath(report.CacheRoot, report.IndexPath),
		cacheIndexStatusLine(report),
		cacheLockStatusLine(report.Lock),
		fmt.Sprintf("Artifacts: %d", report.ArtifactCount),
		"Total size: " + formatByteSize(report.TotalBytes),
		fmt.Sprintf("Stale candidates: %d", report.StaleCandidates),
		fmt.Sprintf("Invalid artifacts: %d", report.InvalidArtifacts),
	}
	for _, entry := range report.Entries {
		lines = append(
			lines,
			fmt.Sprintf(
				"%s %s integrity=%s stale=%t path=%s",
				entry.ResourceID,
				entry.Platform,
				entry.IntegrityStatus,
				entry.Stale,
				formatCachePath(report.CacheRoot, entry.ResolvedPath),
			),
		)
	}
	for _, missing := range report.MissingFiles {
		lines = append(lines, "Missing: "+formatCachePath(report.CacheRoot, missing))
	}
	for _, unexpected := range report.UnexpectedPaths {
		lines = append(lines, "Unexpected: "+formatCachePath(report.CacheRoot, unexpected))
	}

	return writeLines(writer, lines)
}

func cacheConfigStatusLine(report runtimeartifacts.DiagnosticReport) string {
	if report.ConfigError != "" {
		return "Config status: error: " + report.ConfigError
	}
	if report.ConfigExists {
		return fmt.Sprintf("Config status: ok (retention_days=%d)", report.RetentionDays)
	}

	return fmt.Sprintf("Config status: default (retention_days=%d)", report.RetentionDays)
}

func cacheIndexStatusLine(report runtimeartifacts.DiagnosticReport) string {
	if report.IndexError != "" {
		return "Index status: error: " + report.IndexError
	}
	if report.IndexExists {
		return "Index status: ok"
	}

	return "Index status: missing"
}

func cacheLockStatusLine(lock runtimeartifacts.CacheLockStatus) string {
	if lock.Error != "" {
		return "Lock status: error: " + lock.Error
	}
	if lock.Locked {
		return fmt.Sprintf("Lock status: locked (%s)", lock.Mode)
	}

	return "Lock status: unlocked"
}

func writeLines(writer io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}

	return nil
}

// nolint: gochecknoinits
func init() {
	diagCmd.AddCommand(diagCacheCmd)
}
