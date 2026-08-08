// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestResolveInfrastructureConfigVariables_NameBasedEmbeddedPreset(t *testing.T) {
	t.Parallel()

	resolution, err := ResolveInfrastructureConfigVariables(PresetRef{Name: "local"})
	if err != nil {
		t.Fatalf("expected the embedded 'local' preset to resolve, got %v", err)
	}
	if resolution.PresetLabel != "local" {
		t.Fatalf("expected the preset label to be 'local', got %q", resolution.PresetLabel)
	}
	if len(resolution.Variables) == 0 {
		t.Fatal("expected the local preset to declare configurable variables")
	}
}

func TestResolveInfrastructureConfigVariables_UnknownPresetReturnsErrorWithLabel(t *testing.T) {
	t.Parallel()

	resolution, err := ResolveInfrastructureConfigVariables(PresetRef{Name: "does-not-exist"})
	if err == nil {
		t.Fatal("expected an error for an unknown preset")
	}
	if resolution.PresetLabel != "does-not-exist" {
		t.Fatalf(
			"expected the preset label to still be set on error, got %q",
			resolution.PresetLabel,
		)
	}
}

func TestResolveInfrastructureConfigVariables_PathBasedPreset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestFile(t, dir+"/infrastructure.yaml", `
name: Test Infrastructure
description: test infrastructure
backend: local
`)

	resolution, err := ResolveInfrastructureConfigVariables(PresetRef{Path: dir})
	if err != nil {
		t.Fatalf("expected the path-based preset to resolve, got %v", err)
	}
	if resolution.PresetLabel != dir {
		t.Fatalf("expected the preset label to be the path, got %q", resolution.PresetLabel)
	}
}

func TestResolveInfrastructureConfigVariablesFromDeployment_ResolvesLocalBackend(t *testing.T) {
	t.Parallel()

	deployment := newLocalTestDeployment(t)

	resolution, err := ResolveInfrastructureConfigVariablesFromDeployment(deployment)
	if err != nil {
		t.Fatalf("expected the local deployment to resolve, got %v", err)
	}
	if resolution.PresetLabel == "" {
		t.Fatal("expected a non-empty preset label")
	}
	if len(resolution.Variables) == 0 {
		t.Fatal("expected the local backend to declare configurable variables")
	}
}

func TestResolveInfrastructureConfigVariablesFromDeployment_MissingManifestReturnsError(
	t *testing.T,
) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	_, err := ResolveInfrastructureConfigVariablesFromDeployment(deployment)
	if err == nil {
		t.Fatal("expected an error when the infrastructure manifest is missing")
	}
}
