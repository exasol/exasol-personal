// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func TestScanInfrastructurePresetFromArgs_Defaults(t *testing.T) {
	t.Parallel()
	preset, _ := scanInfrastructurePresetSelection([]string{"init"})
	if preset != nil {
		t.Fatalf("expected no preset selection, got: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_PositionalName(t *testing.T) {
	t.Parallel()
	preset, err := scanInfrastructurePresetSelection(
		[]string{"init", presets.DefaultInfrastructure},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInfrastructure || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_PositionalPath(t *testing.T) {
	t.Parallel()
	preset, err := scanInfrastructurePresetSelection([]string{"init", "./my-infra-preset"})
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Path != "./my-infra-preset" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_SkipsUnrelatedFlags(t *testing.T) {
	t.Parallel()
	preset, err := scanInfrastructurePresetSelection(
		[]string{
			"--log-level",
			"debug",
			"init",
			"--deployment-dir",
			"./deploy",
			presets.DefaultInfrastructure,
		},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInfrastructure || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_InstallCommand(t *testing.T) {
	t.Parallel()
	preset, err := scanInfrastructurePresetSelection(
		[]string{"install", presets.DefaultInfrastructure, presets.DefaultInstallation},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInfrastructure || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}
