// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/spf13/cobra"
)

const cacheCmdShortDesc = "Manage the runtime artifact cache"

const cacheCmdLongDesc = cacheCmdShortDesc + `

Runtime artifacts are launcher-managed tools and files that are downloaded on demand
and reused across deployments.
`

var cacheCleanOpts = struct {
	Invalid          bool
	All              bool
	PartialDownloads bool
	DryRun           bool
}{}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: cacheCmdShortDesc,
	Long:  cacheCmdLongDesc,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached runtime artifacts",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		artifactCache, err := runtimeartifacts.NewDefaultCache()
		if err != nil {
			return err
		}
		entries, err := artifactCache.List(cmd.Context())
		if err != nil {
			return err
		}
		if commonFlags.OutputJson {
			return addJSONTerminalOutput(entries)
		}

		return addRenderedTerminalOutput(func(writer io.Writer) error {
			return renderCacheListText(writer, artifactCache.Root(), entries)
		})
	},
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean cached runtime artifacts",
	Long: `Clean cached runtime artifacts.

With no selector, this removes artifacts older than the configured retention period.
Use --invalid to remove artifacts that fail integrity checks.
Use --all to remove every cached runtime artifact.
Use --partial-downloads to remove staged partial downloads.
Use --dry-run to preview a cleanup without removing files.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		if selectedCacheCleanupSelectorCount() > 1 {
			return errors.New("--invalid, --all, and --partial-downloads are mutually exclusive")
		}
		artifactCache, err := runtimeartifacts.NewDefaultCache()
		if err != nil {
			return err
		}
		summary, err := artifactCache.Clean(cmd.Context(), runtimeartifacts.CleanOptions{
			Mode:   selectedCacheCleanupMode(),
			DryRun: cacheCleanOpts.DryRun,
		})
		if err != nil {
			return err
		}

		return addRenderedTerminalOutput(func(writer io.Writer) error {
			return renderCacheCleanText(writer, summary)
		})
	},
}

var cacheUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Clear a stale runtime artifact cache lock",
	Long: `Clear a stale runtime artifact cache lock.

Only use this command when you are certain that no launcher process is currently
using the runtime artifact cache.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		artifactCache, err := runtimeartifacts.NewDefaultCache()
		if err != nil {
			return err
		}
		if err := artifactCache.Unlock(); err != nil {
			return err
		}
		addTerminalOutput("Runtime artifact cache lock cleared.")

		return nil
	},
}

func renderCacheListJSON(
	writer io.Writer,
	entries []runtimeartifacts.CacheEntryInfo,
) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder.Encode(entries)
}

func renderCacheListText(
	writer io.Writer,
	cacheRoot string,
	entries []runtimeartifacts.CacheEntryInfo,
) error {
	if _, err := fmt.Fprintf(writer, "Runtime artifact cache: %s\n", cacheRoot); err != nil {
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(writer, "No cached runtime artifacts.")
		return err
	}
	for _, entry := range entries {
		if _, err := fmt.Fprintf(
			writer,
			"%s %s last_used=%s size=%s path=%s\n",
			entry.ResourceID,
			entry.Platform,
			entry.LastUsedAt.Format("2006-01-02T15:04:05Z07:00"),
			formatByteSize(entry.SizeBytes),
			entry.ResolvedPath,
		); err != nil {
			return err
		}
	}

	return nil
}

func selectedCacheCleanupMode() runtimeartifacts.CleanupMode {
	if cacheCleanOpts.Invalid {
		return runtimeartifacts.CleanupModeInvalid
	}
	if cacheCleanOpts.All {
		return runtimeartifacts.CleanupModeAll
	}
	if cacheCleanOpts.PartialDownloads {
		return runtimeartifacts.CleanupModePartialDownloads
	}

	return runtimeartifacts.CleanupModeStale
}

func selectedCacheCleanupSelectorCount() int {
	count := 0
	if cacheCleanOpts.Invalid {
		count++
	}
	if cacheCleanOpts.All {
		count++
	}
	if cacheCleanOpts.PartialDownloads {
		count++
	}

	return count
}

func renderCacheCleanText(
	writer io.Writer,
	summary runtimeartifacts.CleanSummary,
) error {
	action := "Removed"
	if summary.DryRun {
		action = "Would remove"
	}
	subject := "runtime artifact(s)"
	if summary.Mode == runtimeartifacts.CleanupModePartialDownloads {
		subject = "partial download(s)"
	}
	if _, err := fmt.Fprintf(
		writer,
		"%s %d %s, %s (mode: %s).\n",
		action,
		summary.RemovedEntries,
		subject,
		formatByteSize(summary.RemovedBytes),
		summary.Mode,
	); err != nil {
		return err
	}
	if summary.InvalidEntries > 0 {
		_, err := fmt.Fprintf(writer, "Invalid artifacts: %d\n", summary.InvalidEntries)
		return err
	}

	return nil
}

func formatCachePath(cacheRoot, pathValue string) string {
	if cacheRoot == "" || pathValue == "" {
		return pathValue
	}

	rel, err := filepath.Rel(filepath.Clean(cacheRoot), filepath.Clean(pathValue))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return pathValue
	}

	return filepath.ToSlash(rel)
}

func formatByteSize(sizeBytes int64) string {
	const unit = 1024
	if sizeBytes < unit {
		return fmt.Sprintf("%d B", sizeBytes)
	}

	for _, sizeUnit := range []struct {
		label string
		bytes int64
	}{
		{label: "GB", bytes: unit * unit * unit},
		{label: "MB", bytes: unit * unit},
		{label: "KB", bytes: unit},
	} {
		if sizeBytes >= sizeUnit.bytes {
			if sizeBytes%sizeUnit.bytes == 0 {
				return fmt.Sprintf("%d %s", sizeBytes/sizeUnit.bytes, sizeUnit.label)
			}

			size := float64(sizeBytes) / float64(sizeUnit.bytes)

			return fmt.Sprintf("%.1f %s", size, sizeUnit.label)
		}
	}

	return fmt.Sprintf("%d B", sizeBytes)
}

func registerCacheCommands() {
	cacheListCmd.Flags().SortFlags = false
	registerOutputFlags(cacheListCmd, commonFlags)

	cacheCleanCmd.Flags().BoolVar(
		&cacheCleanOpts.Invalid,
		"invalid",
		false,
		"Remove cached artifacts that fail integrity checks",
	)
	cacheCleanCmd.Flags().BoolVar(
		&cacheCleanOpts.All,
		"all",
		false,
		"Remove all cached runtime artifacts",
	)
	cacheCleanCmd.Flags().BoolVar(
		&cacheCleanOpts.PartialDownloads,
		"partial-downloads",
		false,
		"Remove staged partial downloads",
	)
	cacheCleanCmd.Flags().BoolVar(
		&cacheCleanOpts.DryRun,
		"dry-run",
		false,
		"Preview cleanup without removing files",
	)
	cacheCleanCmd.MarkFlagsMutuallyExclusive("invalid", "all", "partial-downloads")

	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
	cacheCmd.AddCommand(cacheUnlockCmd)
}

// nolint: gochecknoinits
func init() {
	registerCacheCommands()
	rootCmd.AddCommand(cacheCmd)
}
