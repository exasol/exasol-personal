// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//nolint:paralleltest // These tests intentionally exercise Cobra's global rootCmd.
package main

import (
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

// WARNING: Keep the tests in this file sequential.
//
// The preregistration helpers call rootCmd.Find before Cobra's real Parse step.
// Cobra's Find is not read-only: it merges persistent flags and mutates internal
// command flag structures. Running these tests with t.Parallel() races on the
// package-global rootCmd under `go test -race` (and therefore `task tests-unit`).
//
// If these tests ever need parallelism, first stop sharing rootCmd by building a
// fresh command tree per test or by extracting preregistration command discovery
// behind an interface that can be tested without Cobra's global command state.

func TestScanInfrastructurePresetFromArgs_Defaults(t *testing.T) {
	preset, _ := scanInfrastructurePresetSelection([]string{"init"})
	if preset != nil {
		t.Fatalf("expected no preset selection, got: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_PositionalName(t *testing.T) {
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
	preset, err := scanInfrastructurePresetSelection([]string{"init", "./my-infra-preset"})
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Path != "./my-infra-preset" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInfrastructurePresetFromArgs_SkipsUnrelatedFlags(t *testing.T) {
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

func TestScanInstallationPresetFromArgs_DefaultFromInfrastructure(t *testing.T) {
	preset, err := scanInstallationPresetSelection(
		[]string{"install", presets.DefaultInfrastructure},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Name != presets.DefaultInstallation || preset.Path != "" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestScanInstallationPresetFromArgs_ExplicitInstallation(t *testing.T) {
	preset, err := scanInstallationPresetSelection(
		[]string{"install", presets.DefaultInfrastructure, "./install-preset"},
	)
	if err != nil || preset == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preset.Path != "./install-preset" {
		t.Fatalf("unexpected preset: %#v", preset)
	}
}

func TestPreregisteredCommandDetectsConfigSet(t *testing.T) {
	if !preregisteredCommandIs([]string{"config", "set", "--cluster-size", "3"}, configSetCmd) {
		t.Fatal("expected config set command")
	}
}

func TestDeploymentDirFromRawArgs_SeparateFlagValue(t *testing.T) {
	deploymentDir := filepath.Join(t.TempDir(), "deployment")
	deployment, err := deploymentDirFromRawArgs([]string{
		"config",
		"set",
		"--deployment-dir",
		deploymentDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Root() != deploymentDir {
		t.Fatalf("expected deployment dir %q, got %q", deploymentDir, deployment.Root())
	}
}

func TestDeploymentDirFromRawArgs_EqualsFlagValue(t *testing.T) {
	deploymentDir := filepath.Join(t.TempDir(), "deployment")
	deployment, err := deploymentDirFromRawArgs([]string{
		"config",
		"set",
		"--deployment-dir=" + deploymentDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deployment.Root() != deploymentDir {
		t.Fatalf("expected deployment dir %q, got %q", deploymentDir, deployment.Root())
	}
}
