// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"path"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestNewTofuConfigFromPresetDerivesFilePathsFromInfraDir(t *testing.T) {
	t.Parallel()

	infraDir := "/deployment/infrastructure"

	cfg := NewTofuConfigFromPreset(infraDir, presets.InfrastructureTofu{})

	if cfg.WorkDir() != infraDir {
		t.Fatalf("expected work dir %q, got %q", infraDir, cfg.WorkDir())
	}
	if cfg.PlanFile() != path.Join(infraDir, DefaultPlanFile) {
		t.Fatalf("expected default plan file, got %q", cfg.PlanFile())
	}
	if cfg.StateFile() != path.Join(infraDir, DefaultStateFile) {
		t.Fatalf("expected default state file, got %q", cfg.StateFile())
	}
	if cfg.VariablesFile() != path.Join(infraDir, DefaultVariablesFile) {
		t.Fatalf("expected default variables file, got %q", cfg.VariablesFile())
	}
	if cfg.VarsOutputFile() != path.Join(infraDir, DefaultVarsOutput) {
		t.Fatalf("expected default vars output file, got %q", cfg.VarsOutputFile())
	}
}

// Both the variables file and the vars-output file are always joined onto
// workDir, whether the preset supplies a custom relative path or falls back
// to the default — there's no "absolute/verbatim" branch for either.
func TestNewTofuConfigFromPresetHonorsCustomRelativeFilePaths(t *testing.T) {
	t.Parallel()

	infraDir := "/deployment/infrastructure"

	cfg := NewTofuConfigFromPreset(infraDir, presets.InfrastructureTofu{
		VariablesFile:  "custom-variables.tf",
		VarsOutputFile: "custom-vars.tfvars",
	})

	if cfg.VariablesFile() != path.Join(infraDir, "custom-variables.tf") {
		t.Fatalf("expected custom variables file joined onto workDir, got %q", cfg.VariablesFile())
	}
	if cfg.VarsOutputFile() != path.Join(infraDir, "custom-vars.tfvars") {
		t.Fatalf(
			"expected custom vars output file joined onto workDir, got %q",
			cfg.VarsOutputFile(),
		)
	}
}
