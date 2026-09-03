// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestSelectedPresetUseLine_KeepsInstallationPresetOptional(t *testing.T) {
	t.Parallel()

	// Given only an infrastructure preset is selected
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "local"},
		InfrastructureLabel: "local",
	}

	// When the usage line is built
	useLine := selectedPresetUseLine("install", selection)

	// Then the selected preset replaces the infrastructure placeholder
	if useLine != "install local [install preset name-or-path]" {
		t.Fatalf("unexpected usage line: %q", useLine)
	}
}

func TestSelectedPresetUseLine_ShowsBothSelectedPresets(t *testing.T) {
	t.Parallel()

	// Given both presets are selected
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "aws"},
		InfrastructureLabel: "aws",
		Installation:        &deploy.PresetRef{Name: "ubuntu"},
		InstallationLabel:   "ubuntu",
	}

	// When the usage line is built
	useLine := selectedPresetUseLine("init", selection)

	// Then both selected presets appear and no placeholder remains
	if useLine != "init aws ubuntu" {
		t.Fatalf("unexpected usage line: %q", useLine)
	}
}

func TestSelectedPresetUseLine_IdentifiesPathPresetsByPath(t *testing.T) {
	t.Parallel()

	// Given a preset selected by a relative directory path, resolved to an absolute one
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Path: "/resolved/elsewhere/my-preset"},
		InfrastructureLabel: "./my-preset",
	}

	// When the usage line is built
	useLine := selectedPresetUseLine("install", selection)

	// Then the path the user typed identifies the preset, not the resolved one
	if !strings.HasPrefix(useLine, "install ./my-preset ") {
		t.Fatalf("unexpected usage line: %q", useLine)
	}
}

func TestSelectedPresetUseLine_QuotesAnArgumentContainingSpaces(t *testing.T) {
	t.Parallel()

	// Given a preset selected by a path containing a space
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Path: "/tmp/my preset"},
		InfrastructureLabel: "/tmp/my preset",
	}

	// When the usage line is built
	useLine := selectedPresetUseLine("install", selection)

	// Then the path stays a single argument
	if !strings.HasPrefix(useLine, `install "/tmp/my preset" `) {
		t.Fatalf("unexpected usage line: %q", useLine)
	}
}

func TestSelectedPresetExamples_QuoteAnArgumentContainingSpaces(t *testing.T) {
	t.Parallel()

	// Given a preset selected by a path containing a space
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Path: "/tmp/my preset"},
		InfrastructureLabel: "/tmp/my preset",
	}

	// When the examples are built
	examples := selectedPresetExamples("install", selection, []string{"ubuntu"})

	// Then every example keeps the path as a single argument
	want := "  exasol install \"/tmp/my preset\"\n" +
		"  exasol install \"/tmp/my preset\" ubuntu"
	if examples != want {
		t.Fatalf("unexpected examples:\n%s", examples)
	}
}

func TestSelectedPresetExamples_SuggestsACompatibleInstallationPreset(t *testing.T) {
	t.Parallel()

	// Given an infrastructure preset with compatible installation presets
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "aws"},
		InfrastructureLabel: "aws",
	}

	// When the examples are built
	examples := selectedPresetExamples("install", selection, []string{"ubuntu", "other"})

	// Then the selected preset is shown alone and paired with the first compatible preset
	want := "  exasol install aws\n  exasol install aws ubuntu"
	if examples != want {
		t.Fatalf("unexpected examples:\n%s", examples)
	}
}

func TestSelectedPresetExamples_OmitsThePairWithoutACompatiblePreset(t *testing.T) {
	t.Parallel()

	// Given an infrastructure preset without compatible installation presets
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "local"},
		InfrastructureLabel: "local",
	}

	// When the examples are built
	examples := selectedPresetExamples("install", selection, nil)

	// Then only the infrastructure preset is exemplified
	if examples != "  exasol install local" {
		t.Fatalf("unexpected examples:\n%s", examples)
	}
}

func TestSelectedPresetExamples_UsesBothSelectedPresets(t *testing.T) {
	t.Parallel()

	// Given both presets are selected
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "aws"},
		InfrastructureLabel: "aws",
		Installation:        &deploy.PresetRef{Name: "ubuntu"},
		InstallationLabel:   "ubuntu",
	}

	// When the examples are built
	examples := selectedPresetExamples("install", selection, []string{"ubuntu"})

	// Then the selected pair is the only example
	if examples != "  exasol install aws ubuntu" {
		t.Fatalf("unexpected examples:\n%s", examples)
	}
}

func TestCompatibleInstallationPresetsForHelp_ListsPresetNames(t *testing.T) {
	t.Parallel()

	// Given compatible installation presets
	// When they are rendered for help
	rendered := compatibleInstallationPresetsForHelp([]string{"local", "ubuntu"})

	// Then they are listed
	if rendered != "local, ubuntu" {
		t.Fatalf("unexpected compatible presets: %q", rendered)
	}
}

func TestCompatibleInstallationPresetsForHelp_ReportsNone(t *testing.T) {
	t.Parallel()

	// Given no compatible installation preset
	// When the compatible presets are rendered for help
	rendered := compatibleInstallationPresetsForHelp(nil)

	// Then the absence is stated rather than left blank
	if rendered != "none" {
		t.Fatalf("unexpected compatible presets: %q", rendered)
	}
}

func TestRenderSelectedPresetHelp_DescribesTheInfrastructurePreset(t *testing.T) {
	t.Parallel()

	// Given a selected infrastructure preset with a description
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "local"},
		InfrastructureLabel: "local",
	}
	manifest := &presets.InfrastructureManifest{
		Name:        "Exasol Local",
		Description: "Exasol Local deployment.",
	}

	// When the preset help is rendered
	rendered, err := renderSelectedPresetHelp(
		t.Context(), selection, manifest, []string{"local"},
	)
	if err != nil {
		t.Fatalf("rendering the selected preset help failed: %v", err)
	}

	// Then it identifies the preset, describes it, and names its compatible presets
	for _, want := range []string{
		"Infrastructure preset `local`:",
		"Exasol Local deployment.",
		"Compatible installation presets: local",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in rendered help:\n%s", want, rendered)
		}
	}
}

func TestRenderSelectedPresetHelp_OmitsAnEmptyDescriptionLine(t *testing.T) {
	t.Parallel()

	// Given a selected preset whose manifest carries no description
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "local"},
		InfrastructureLabel: "local",
	}
	manifest := &presets.InfrastructureManifest{Name: "Exasol Local"}

	// When the preset help is rendered
	rendered, err := renderSelectedPresetHelp(
		t.Context(), selection, manifest, []string{"local"},
	)
	if err != nil {
		t.Fatalf("rendering the selected preset help failed: %v", err)
	}

	// Then no blank description line separates the preset from its compatibility line
	if strings.Contains(rendered, "\n\t  \n") {
		t.Fatalf("expected no blank description line in rendered help:\n%q", rendered)
	}
}

func TestRenderSelectedPresetHelp_IdentifiesAPresetByTheArgumentAsTyped(t *testing.T) {
	t.Parallel()

	// Given a preset selected by a relative path, resolved to an absolute one
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Path: "/resolved/elsewhere/my-preset"},
		InfrastructureLabel: "./my-preset",
	}
	manifest := &presets.InfrastructureManifest{Description: "A preset of my own."}

	// When the preset help is rendered
	rendered, err := renderSelectedPresetHelp(t.Context(), selection, manifest, nil)
	if err != nil {
		t.Fatalf("rendering the selected preset help failed: %v", err)
	}

	// Then the help names the path the user typed, not the resolved location
	if !strings.Contains(rendered, "Infrastructure preset `./my-preset`:") {
		t.Fatalf("expected the typed path in rendered help:\n%s", rendered)
	}
	if strings.Contains(rendered, "/resolved/elsewhere") {
		t.Fatalf("expected no resolved path in rendered help:\n%s", rendered)
	}
}

func TestRenderSelectedPresetHelp_FailsWhenAnInstallationPresetCannotBeDescribed(
	t *testing.T,
) {
	t.Parallel()

	// Given a selected installation preset whose manifest cannot be read
	selection := presetHelpSelection{
		Infrastructure:      deploy.PresetRef{Name: "aws"},
		InfrastructureLabel: "aws",
		Installation:        &deploy.PresetRef{Path: "/nonexistent/preset/directory"},
		InstallationLabel:   "/nonexistent/preset/directory",
	}
	manifest := &presets.InfrastructureManifest{Description: "Provisioning in AWS."}

	// When the preset help is rendered
	rendered, err := renderSelectedPresetHelp(t.Context(), selection, manifest, []string{"ubuntu"})

	// Then rendering fails rather than describing only one of the selected presets
	if err == nil {
		t.Fatalf("expected rendering to fail, got:\n%s", rendered)
	}
	if rendered != "" {
		t.Fatalf("expected no partial help, got:\n%s", rendered)
	}
}

// nolint: paralleltest
func TestPreparePresetHelpSelection_DoesNotScanWithoutAHelpRequest(t *testing.T) {
	// Do not run in parallel: this test reads and restores the package-level selection.
	t.Cleanup(func() { selectedPresetHelp = nil })
	selectedPresetHelp = nil

	// Given a normal command invocation that does not request help
	args := []string{"install", "local", "--deployment-dir", "/tmp/deployment"}

	// When the preset help selection is prepared
	// (a context without a resource manager: resolving a preset would panic or fail,
	// so reaching the resolver at all is what this test rules out)
	preparePresetHelpSelection(t.Context(), args)

	// Then no selection is recorded and no preset was resolved
	if selectedPresetHelp != nil {
		t.Fatalf("expected no preset help selection, got %+v", selectedPresetHelp)
	}
}
