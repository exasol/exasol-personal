// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

// deferPresetHelpText appends the embedded preset name list and compatibility
// matrix to cmd's Long text the first time its help is actually requested,
// rather than unconditionally at process startup (package init() runs for
// every invocation regardless of which subcommand is selected, and has no
// request context to resolve presets with): computing either resolves every
// embedded preset through the resource cache, which bumps each preset's
// last-used bookkeeping as a side effect.
func deferPresetHelpText(cmd *cobra.Command) {
	defaultHelpFunc := cmd.HelpFunc()
	appended := false
	cmd.SetHelpFunc(func(helpCmd *cobra.Command, args []string) {
		if !appended {
			ctx := helpCmd.Context()
			helpCmd.Long = strings.TrimRight(helpCmd.Long, "\n") +
				"\n\t" + presetNamesForHelp(
				presets.PresetTypeInfrastructure,
				presets.ListEmbeddedPresets(ctx, presets.Infrastructure),
			) +
				"\n\t" + presetNamesForHelp(
				presets.PresetTypeInstallation,
				presets.ListEmbeddedPresets(ctx, presets.Installation),
			)
			if matrix := embeddedPresetCompatibilityMatrix(ctx); matrix != "" {
				helpCmd.Long = strings.TrimRight(helpCmd.Long, "\n") +
					"\n\n\t" + strings.ReplaceAll(matrix, "\n", "\n\t")
			}
			appended = true
		}
		defaultHelpFunc(helpCmd, args)
	})
}

func embeddedPresetCompatibilityMatrix(ctx context.Context) string {
	infraIDs := presets.ListEmbeddedPresets(ctx, presets.Infrastructure)
	installIDs := presets.ListEmbeddedPresets(ctx, presets.Installation)
	if len(infraIDs) == 0 || len(installIDs) == 0 {
		return ""
	}

	infraManifests := readEmbeddedInfrastructureManifests(ctx, infraIDs)
	installManifests := readEmbeddedInstallationManifests(ctx, installIDs)
	if len(infraManifests) == 0 || len(installManifests) == 0 {
		return ""
	}

	const compatibleCell = "yes"
	matrixCtx := compatibilityMatrixContext{
		installIDs:       installIDs,
		installManifests: installManifests,
		columnWidths:     compatibilityMatrixColumnWidths(installManifests, compatibleCell),
		firstColumnWidth: compatibilityMatrixFirstColumnWidth(infraManifests),
		compatibleCell:   compatibleCell,
	}

	var builder strings.Builder
	_, _ = builder.WriteString("Compatibility matrix (embedded presets):\n")
	writeCompatibilityMatrixHeader(&builder, matrixCtx)

	for _, infraID := range infraIDs {
		infraManifest, ok := infraManifests[infraID]
		if !ok {
			continue
		}
		writeCompatibilityMatrixRow(&builder, matrixCtx, infraID, infraManifest)
	}

	return strings.TrimRight(builder.String(), "\n")
}

// compatibilityMatrixContext holds the rendering context shared by the
// compatibility matrix's header and every row.
type compatibilityMatrixContext struct {
	installIDs       []string
	installManifests map[string]*presets.InstallManifest
	columnWidths     map[string]int
	firstColumnWidth int
	compatibleCell   string
}

func readEmbeddedInfrastructureManifests(
	ctx context.Context,
	infraIDs []string,
) map[string]*presets.InfrastructureManifest {
	manifests := map[string]*presets.InfrastructureManifest{}
	for _, infraID := range infraIDs {
		manifest, err := presets.ReadInfrastructureManifest(ctx, infraID)
		if err != nil {
			continue
		}
		manifests[infraID] = manifest
	}

	return manifests
}

func readEmbeddedInstallationManifests(
	ctx context.Context,
	installIDs []string,
) map[string]*presets.InstallManifest {
	manifests := map[string]*presets.InstallManifest{}
	for _, installID := range installIDs {
		manifest, err := presets.ReadInstallManifest(ctx, installID)
		if err != nil {
			continue
		}
		manifests[installID] = manifest
	}

	return manifests
}

func compatibilityMatrixFirstColumnWidth(
	infraManifests map[string]*presets.InfrastructureManifest,
) int {
	width := len("infrastructure")
	for infraID := range infraManifests {
		if len(infraID) > width {
			width = len(infraID)
		}
	}

	return width
}

func compatibilityMatrixColumnWidths(
	installManifests map[string]*presets.InstallManifest, compatibleCell string,
) map[string]int {
	widths := map[string]int{}
	for installID := range installManifests {
		width := max(len(installID), len(compatibleCell))
		widths[installID] = width
	}

	return widths
}

func writeCompatibilityMatrixHeader(builder *strings.Builder, ctx compatibilityMatrixContext) {
	_, _ = fmt.Fprintf(builder, "  %-*s", ctx.firstColumnWidth, "infrastructure")
	for _, installID := range ctx.installIDs {
		if _, ok := ctx.installManifests[installID]; !ok {
			continue
		}
		_, _ = fmt.Fprintf(builder, "  %-*s", ctx.columnWidths[installID], installID)
	}
	_ = builder.WriteByte('\n')
}

func writeCompatibilityMatrixRow(
	builder *strings.Builder,
	ctx compatibilityMatrixContext,
	infraID string,
	infraManifest *presets.InfrastructureManifest,
) {
	_, _ = fmt.Fprintf(builder, "  %-*s", ctx.firstColumnWidth, infraID)
	for _, installID := range ctx.installIDs {
		installManifest, ok := ctx.installManifests[installID]
		if !ok {
			continue
		}
		cell := "no"
		if embeddedPresetPairCompatible(infraManifest, installManifest) {
			cell = ctx.compatibleCell
		}
		_, _ = fmt.Fprintf(builder, "  %-*s", ctx.columnWidths[installID], cell)
	}
	_ = builder.WriteByte('\n')
}

func embeddedPresetPairCompatible(
	infrastructureManifest *presets.InfrastructureManifest,
	installationManifest *presets.InstallManifest,
) bool {
	if infrastructureManifest == nil || installationManifest == nil {
		return false
	}

	required := installationManifest.RequiredCapabilities()
	if len(required) == 0 {
		return true
	}

	providedSet := map[string]struct{}{}
	for _, capability := range infrastructureManifest.ProvidedCapabilities() {
		providedSet[capability] = struct{}{}
	}

	for _, capability := range required {
		if _, ok := providedSet[capability]; !ok {
			return false
		}
	}

	return true
}
