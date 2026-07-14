// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

// TestEnsureDeploymentPresetIdentityMatches_BackfillsAndPersistsOldStyleDeployment is a
// regression test for splitting persistence out of resolvePresetIdentity (formerly
// loadOrBackfillPresetIdentity): EnsureDeploymentPresetIdentityMatches must still
// backfill and persist identically to before the split.
func TestEnsureDeploymentPresetIdentityMatches_BackfillsAndPersistsOldStyleDeployment(
	t *testing.T,
) {
	t.Parallel()

	// Given a deployment initialized by a launcher version that predates
	// persisted preset identity.
	deployment := initializedDeploymentOrFail(t)
	clearPersistedPresetIdentity(t, deployment)

	// When the same preset is requested again.
	err := EnsureDeploymentPresetIdentityMatches(
		deployment,
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
	)
	// Then it succeeds and persists the backfilled identity to disk.
	if err != nil {
		t.Fatalf("expected backfilled identity to match, got %v", err)
	}
	state, readErr := config.ReadExasolPersonalState(deployment)
	if readErr != nil {
		t.Fatalf("read state failed: %v", readErr)
	}
	if state.InfrastructurePresetIdentity == "" || state.InstallationPresetIdentity == "" {
		t.Fatal("expected backfilled preset identity to be persisted to disk")
	}
}

// TestResolveDeploymentPresetIdentity_DerivesForOldStyleDeploymentWithoutWriting verifies
// the new read-only entry point (used by `exasol deployments list`) derives the same
// display identity for an old-style deployment without ever writing to its state file.
func TestResolveDeploymentPresetIdentity_DerivesForOldStyleDeploymentWithoutWriting(
	t *testing.T,
) {
	t.Parallel()

	// Given a deployment initialized by a launcher version that predates
	// persisted preset identity.
	deployment := initializedDeploymentOrFail(t)
	clearPersistedPresetIdentity(t, deployment)

	// When its preset identity is resolved for display.
	identity, err := ResolveDeploymentPresetIdentity(deployment)
	// Then it derives a non-empty identity...
	if err != nil {
		t.Fatalf("expected identity resolution to succeed, got %v", err)
	}
	if identity.Infrastructure == "" || identity.Installation == "" {
		t.Fatal("expected derived preset identity to be non-empty")
	}

	// ...and does not persist it.
	state, readErr := config.ReadExasolPersonalState(deployment)
	if readErr != nil {
		t.Fatalf("read state failed: %v", readErr)
	}
	if state.InfrastructurePresetIdentity != "" || state.InstallationPresetIdentity != "" {
		t.Fatal("expected ResolveDeploymentPresetIdentity not to persist the derived identity")
	}
}

func initializedDeploymentOrFail(t *testing.T) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: presets.DefaultInfrastructure},
		PresetRef{Name: presets.DefaultInstallation},
		map[string]string{},
		map[string]string{},
		deployment,
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	return deployment
}

func clearPersistedPresetIdentity(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		t.Fatalf("read state failed: %v", err)
	}
	state.InfrastructurePresetIdentity = ""
	state.InstallationPresetIdentity = ""
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		t.Fatalf("write state failed: %v", err)
	}
}
