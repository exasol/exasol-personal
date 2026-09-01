// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/resource"
	"github.com/spf13/cobra"
)

const cacheCmdShortDesc = "Manage the resource cache"

const cacheCmdLongDesc = cacheCmdShortDesc + `

Resources are launcher-managed tools and files that are downloaded on demand
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
	Short: "List cached resources",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		artifactCache, err := resource.NewDefaultCache()
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
	Short: "Clean cached resources",
	Long: `Clean cached resources.

With no selector, this removes artifacts older than the configured retention period.
Use --invalid to remove artifacts that fail integrity checks.
Use --all to remove every cached resource.
Use --partial-downloads to remove staged partial downloads.
Use --dry-run to preview a cleanup without removing files.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		if selectedCacheCleanupSelectorCount() > 1 {
			return errors.New("--invalid, --all, and --partial-downloads are mutually exclusive")
		}
		artifactCache, err := resource.NewDefaultCache()
		if err != nil {
			return err
		}
		summary, err := artifactCache.Clean(cmd.Context(), resource.CleanOptions{
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
	Short: "Clear a stale resource cache lock",
	Long: `Clear a stale resource cache lock.

Only use this command when you are certain that no launcher process is currently
using the resource cache.
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		artifactCache, err := resource.NewDefaultCache()
		if err != nil {
			return err
		}
		if err := artifactCache.Unlock(); err != nil {
			return err
		}
		addTerminalNotice("Resource cache lock cleared.")

		return nil
	},
}

func renderCacheListText(
	writer io.Writer,
	cacheRoot string,
	entries []resource.CacheEntryInfo,
) error {
	if _, err := fmt.Fprintf(writer, "Resource cache: %s\n", cacheRoot); err != nil {
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(writer, "No cached resources.")
		return err
	}
	for _, entry := range entries {
		if _, err := fmt.Fprintf(
			writer,
			"%s last_used=%s size=%s path=%s\n",
			strings.Join(entry.ResourceIDs, ","),
			entry.LastUsedAt.Format("2006-01-02T15:04:05Z07:00"),
			formatByteSize(entry.SizeBytes),
			entry.Path,
		); err != nil {
			return err
		}
	}

	return nil
}

func selectedCacheCleanupMode() resource.CleanupMode {
	if cacheCleanOpts.Invalid {
		return resource.CleanupModeInvalid
	}
	if cacheCleanOpts.All {
		return resource.CleanupModeAll
	}
	if cacheCleanOpts.PartialDownloads {
		return resource.CleanupModePartialDownloads
	}

	return resource.CleanupModeStale
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
	summary resource.CleanSummary,
) error {
	action := "Removed"
	if summary.DryRun {
		action = "Would remove"
	}
	subject := "resource(s)"
	if summary.Mode == resource.CleanupModePartialDownloads {
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
		"Remove all cached resources",
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
