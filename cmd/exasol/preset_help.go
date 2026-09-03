// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/spf13/cobra"
)

// presetCommandLongDesc assembles the long help of a command that takes preset arguments:
// the command's own description, then a section about the presets, then the tip pointing at
// preset discovery. Both the generic and the preset-specific help are built through here so
// the order of the sections has one owner.
func presetCommandLongDesc(baseDesc, presetSection string) string {
	return baseDesc + presetSection + presetDiscoveryTipHelp
}

// presetHelpSelection records the presets selected on the command line. Installation is
// set only when a second preset argument selects it explicitly.
//
// The labels are the arguments as typed. Resolving a preset canonicalizes a local path and
// resolves a remote preset into the artifact cache, neither of which the user wrote, so the
// resolved reference is used to read manifests while the label is what help echoes back.
type presetHelpSelection struct {
	Infrastructure      deploy.PresetRef
	InfrastructureLabel string
	Installation        *deploy.PresetRef
	InstallationLabel   string
}

// selectedPresetHelp holds the preset selection scanned from the raw arguments before Cobra
// parses them. It is recorded separately from the preset variable flags because registering
// those stops early for a preset that declares no variables, which would leave such a preset
// looking unselected.
var selectedPresetHelp *presetHelpSelection

// preparePresetHelpSelection records the preset selection that preset-specific help
// describes. A selection that cannot be resolved leaves the generic help in place.
//
// Only an explicit help request scans: resolving a preset argument reaches the resource
// manager, which re-fetches an artifact that carries no checksum, so scanning
// unconditionally would add a download to every normal init or deploy.
func preparePresetHelpSelection(ctx context.Context, args []string) {
	if !rawArgsRequestHelp(args) {
		return
	}

	positionals, err := scanPresetPositionals(args)
	if err != nil || len(positionals) == 0 {
		return
	}

	infrastructure, err := resolvePresetRef(
		ctx, positionals[infrastructurePresetArgIndex], presets.PresetTypeInfrastructure,
	)
	if err != nil {
		return
	}

	selection := &presetHelpSelection{
		Infrastructure:      infrastructure,
		InfrastructureLabel: strings.TrimSpace(positionals[infrastructurePresetArgIndex]),
	}
	if len(positionals) > installationPresetArgIndex {
		installationArg := positionals[installationPresetArgIndex]
		installation, err := resolvePresetRef(
			ctx, installationArg, presets.PresetTypeInstallation,
		)
		if err != nil {
			return
		}
		selection.Installation = &installation
		selection.InstallationLabel = strings.TrimSpace(installationArg)
	}
	selectedPresetHelp = selection
}

// deferPresetHelpText completes cmd's Long text the first time its help is actually
// requested, rather than unconditionally at process startup (package init() runs for
// every invocation regardless of which subcommand is selected, and has no
// request context to resolve presets with): resolving presets goes through the resource
// cache, which bumps each preset's last-used bookkeeping as a side effect.
//
// baseDesc is the part of the long description that preset-specific help keeps.
func deferPresetHelpText(cmd *cobra.Command, baseDesc string) {
	defaultHelpFunc := cmd.HelpFunc()
	applied := false
	cmd.SetHelpFunc(func(helpCmd *cobra.Command, args []string) {
		if !applied {
			applyPresetHelpText(helpCmd, baseDesc)
			applied = true
		}
		defaultHelpFunc(helpCmd, args)
	})
}

func applyPresetHelpText(cmd *cobra.Command, baseDesc string) {
	ctx := cmd.Context()
	if selectedPresetHelp != nil &&
		applySelectedPresetHelp(ctx, cmd, baseDesc, *selectedPresetHelp) {
		return
	}

	appendGenericPresetHelp(ctx, cmd)
}

// applySelectedPresetHelp describes the selected presets in place of the overview of every
// embedded preset. It reports whether the selection could be described.
func applySelectedPresetHelp(
	ctx context.Context,
	cmd *cobra.Command,
	baseDesc string,
	selection presetHelpSelection,
) bool {
	manifest, err := readInfrastructureManifestForRef(ctx, selection.Infrastructure)
	if err != nil {
		return false
	}

	compatible := compatibleInstallationPresetNames(ctx, manifest)
	body, err := renderSelectedPresetHelp(ctx, selection, manifest, compatible)
	if err != nil {
		return false
	}

	cmd.Long = presetCommandLongDesc(baseDesc, body)
	cmd.Use = selectedPresetUseLine(cmd.Name(), selection)
	cmd.Example = selectedPresetExamples(cmd.Name(), selection, compatible)

	return true
}

func appendGenericPresetHelp(ctx context.Context, cmd *cobra.Command) {
	cmd.Long = strings.TrimRight(cmd.Long, "\n") +
		"\n\t" + presetNamesForHelp(
		presets.PresetTypeInfrastructure,
		presets.ListEmbeddedPresets(ctx, presets.Infrastructure),
	) +
		"\n\t" + presetNamesForHelp(
		presets.PresetTypeInstallation,
		presets.ListEmbeddedPresets(ctx, presets.Installation),
	)
	if matrix := embeddedPresetCompatibilityMatrix(ctx); matrix != "" {
		cmd.Long = strings.TrimRight(cmd.Long, "\n") +
			"\n\n\t" + strings.ReplaceAll(matrix, "\n", "\n\t")
	}
}

// renderSelectedPresetHelp describes the selected presets. It fails when a selected preset
// cannot be described, so that help falls back to the overview of every preset rather than
// omitting one preset's description while the usage line still advertises it.
func renderSelectedPresetHelp(
	ctx context.Context,
	selection presetHelpSelection,
	infraManifest *presets.InfrastructureManifest,
	compatible []string,
) (string, error) {
	var builder strings.Builder
	_ = builder.WriteByte('\n')
	writePresetHelpEntry(
		&builder,
		"Infrastructure",
		selection.InfrastructureLabel,
		infraManifest.Description,
	)
	_, _ = fmt.Fprintf(
		&builder,
		"\t  Compatible installation presets: %s\n",
		compatibleInstallationPresetsForHelp(compatible),
	)

	if selection.Installation == nil {
		return builder.String(), nil
	}

	installManifest, err := readInstallManifestForRef(ctx, *selection.Installation)
	if err != nil {
		return "", err
	}
	_ = builder.WriteByte('\n')
	writePresetHelpEntry(
		&builder,
		"Installation",
		selection.InstallationLabel,
		installManifest.Description,
	)

	return builder.String(), nil
}

func writePresetHelpEntry(builder *strings.Builder, kind, label, description string) {
	_, _ = fmt.Fprintf(builder, "\t%s preset `%s`:\n", kind, label)
	if strings.TrimSpace(description) != "" {
		_, _ = fmt.Fprintf(builder, "\t  %s\n", strings.TrimSpace(description))
	}
}

// compatibleInstallationPresetNames lists the embedded installation presets the selected
// infrastructure preset provides the required capabilities for, using the same comparison
// that builds the compatibility matrix.
func compatibleInstallationPresetNames(
	ctx context.Context, infraManifest *presets.InfrastructureManifest,
) []string {
	installIDs := presets.ListEmbeddedPresets(ctx, presets.Installation)
	installManifests := readEmbeddedInstallationManifests(ctx, installIDs)

	compatible := make([]string, 0, len(installIDs))
	for _, installID := range installIDs {
		installManifest, ok := installManifests[installID]
		if !ok {
			continue
		}
		if embeddedPresetPairCompatible(infraManifest, installManifest) {
			compatible = append(compatible, installID)
		}
	}

	return compatible
}

func compatibleInstallationPresetsForHelp(compatible []string) string {
	if len(compatible) == 0 {
		return "none"
	}

	return strings.Join(compatible, ", ")
}

func selectedPresetUseLine(commandName string, selection presetHelpSelection) string {
	infraArg := shellQuotedPresetArg(selection.InfrastructureLabel)
	if selection.Installation != nil {
		return fmt.Sprintf(
			"%s %s %s",
			commandName, infraArg, shellQuotedPresetArg(selection.InstallationLabel),
		)
	}

	return fmt.Sprintf("%s %s [install preset name-or-path]", commandName, infraArg)
}

func selectedPresetExamples(
	commandName string, selection presetHelpSelection, compatible []string,
) string {
	infraArg := shellQuotedPresetArg(selection.InfrastructureLabel)
	if selection.Installation != nil {
		return fmt.Sprintf(
			"  exasol %s %s %s",
			commandName, infraArg, shellQuotedPresetArg(selection.InstallationLabel),
		)
	}

	example := fmt.Sprintf("  exasol %s %s", commandName, infraArg)
	if len(compatible) == 0 {
		return example
	}

	return example + fmt.Sprintf(
		"\n  exasol %s %s %s", commandName, infraArg, shellQuotedPresetArg(compatible[0]),
	)
}

// shellQuotedPresetArg wraps a preset argument in double quotes when it contains
// whitespace, so that an example stays one argument when it is copied into a shell. Double
// quotes group an argument in POSIX shells, cmd.exe, and PowerShell alike.
func shellQuotedPresetArg(arg string) string {
	if !strings.ContainsAny(arg, " \t") {
		return arg
	}

	return `"` + arg + `"`
}

func readInfrastructureManifestForRef(
	ctx context.Context, ref deploy.PresetRef,
) (*presets.InfrastructureManifest, error) {
	if ref.IsPath() {
		return presets.ReadInfrastructureManifestFromDir(ref.Path)
	}

	return presets.ReadInfrastructureManifest(ctx, ref.Name)
}

func readInstallManifestForRef(
	ctx context.Context, ref deploy.PresetRef,
) (*presets.InstallManifest, error) {
	if ref.IsPath() {
		return presets.ReadInstallManifestFromDir(ref.Path)
	}

	return presets.ReadInstallManifest(ctx, ref.Name)
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
