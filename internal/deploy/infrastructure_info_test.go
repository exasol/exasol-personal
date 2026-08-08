// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestGetInfrastructureInfo_ResolvesEmbeddedPreset(t *testing.T) {
	t.Parallel()

	info, err := GetInfrastructureInfo(presets.DefaultInfrastructure)
	if err != nil {
		t.Fatalf("expected the embedded preset to resolve, got %v", err)
	}
	if info.Name == "" {
		t.Fatal("expected a non-empty infrastructure name")
	}
	if info.ShortDescription == "" {
		t.Fatal("expected a non-empty short description")
	}
}

func TestGetInfrastructureInfo_UnknownPresetReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := GetInfrastructureInfo("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown infrastructure preset")
	}
}

func TestGetInfrastructureInfoFromDir_ResolvesPathBasedPreset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestFile(t, dir+"/infrastructure.yaml", `
name: Test Infrastructure
description: test infrastructure
backend: local
`)

	info, err := GetInfrastructureInfoFromDir(dir)
	if err != nil {
		t.Fatalf("expected the path-based preset to resolve, got %v", err)
	}
	if info.Name != "Test Infrastructure" {
		t.Fatalf("expected the manifest name, got %q", info.Name)
	}
	if info.LongDescription != "test infrastructure" {
		t.Fatalf("expected the manifest description, got %q", info.LongDescription)
	}
}

func TestGetInfrastructureInfoFromDir_MissingManifestReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := GetInfrastructureInfoFromDir(t.TempDir()); err == nil {
		t.Fatal("expected an error when the infrastructure manifest is missing")
	}
}

func TestGetInfrastructureInfoFromPreset_NameBasedRef(t *testing.T) {
	t.Parallel()

	info, err := GetInfrastructureInfoFromPreset(PresetRef{Name: presets.DefaultInfrastructure})
	if err != nil {
		t.Fatalf("expected the embedded preset to resolve, got %v", err)
	}
	if info.Name == "" {
		t.Fatal("expected a non-empty infrastructure name")
	}
}

func TestGetInfrastructureInfoFromPreset_PathBasedRef(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestFile(t, dir+"/infrastructure.yaml", `
name: Test Infrastructure
description: test infrastructure
backend: local
`)

	info, err := GetInfrastructureInfoFromPreset(PresetRef{Path: dir})
	if err != nil {
		t.Fatalf("expected the path-based preset to resolve, got %v", err)
	}
	if info.Name != "Test Infrastructure" {
		t.Fatalf("expected the manifest name, got %q", info.Name)
	}
}
