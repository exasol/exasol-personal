// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

// presetIdentityOf returns a stable selector for a preset reference, suitable
// for persisting in launcher state and for equality comparisons.
//
// Format: "name:<embedded-preset-name>" or "path:<absolute-path>".
func presetIdentityOf(preset PresetRef) string {
	if preset.IsPath() {
		path := filepath.Clean(strings.TrimSpace(preset.Path))
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}

		return "path:" + path
	}

	return "name:" + strings.TrimSpace(preset.Name)
}

// presetIdentityDisplay strips the kind prefix from a stored identity for
// human-readable error messages.
func presetIdentityDisplay(identity string) string {
	for _, prefix := range []string{"name:", "path:"} {
		if rest, ok := strings.CutPrefix(identity, prefix); ok {
			return rest
		}
	}

	return identity
}

// EnsureDeploymentPresetIdentityMatches verifies the requested presets match
// the presets the deployment directory was initialized with. For deployments
// initialized by older launcher versions without persisted identity, it
// backfills the identity from extracted manifests and persists the result.
func EnsureDeploymentPresetIdentityMatches(
	deployment config.DeploymentDir,
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
) error {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}
	existing, err := loadOrBackfillPresetIdentity(state, deployment)
	if err != nil {
		return err
	}

	requested := presetIdentityPair{
		infrastructure: presetIdentityOf(infrastructurePreset),
		installation:   presetIdentityOf(installationPreset),
	}
	if existing == requested {
		slog.Info("deployment directory is already initialized with the requested presets")

		return nil
	}

	return fmt.Errorf(
		"%w: existing infrastructure %q, installation %q; "+
			"requested infrastructure %q, installation %q.\n"+
			"Run `exasol destroy --remove` before initializing different presets, "+
			"or run `exasol remove` if the deployment resources are already gone",
		ErrDeploymentPresetMismatch,
		presetIdentityDisplay(existing.infrastructure),
		presetIdentityDisplay(existing.installation),
		presetIdentityDisplay(requested.infrastructure),
		presetIdentityDisplay(requested.installation),
	)
}

// presetIdentityPair groups the persisted preset identity strings for a
// deployment so they can be returned and compared as a unit.
type presetIdentityPair struct {
	infrastructure string
	installation   string
}

// loadOrBackfillPresetIdentity returns the persisted preset identity pair.
// For deployments initialized by older launcher versions that lack the
// identity fields, it backfills from extracted manifests and persists.
func loadOrBackfillPresetIdentity(
	state *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) (presetIdentityPair, error) {
	if state.InfrastructurePresetIdentity != "" && state.InstallationPresetIdentity != "" {
		return presetIdentityPair{
			infrastructure: state.InfrastructurePresetIdentity,
			installation:   state.InstallationPresetIdentity,
		}, nil
	}

	infraManifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return presetIdentityPair{}, fmt.Errorf("backfill preset identity: %w", err)
	}
	installManifest, err := config.ReadInstallManifest(deployment)
	if err != nil {
		return presetIdentityPair{}, fmt.Errorf("backfill preset identity: %w", err)
	}

	pair := presetIdentityPair{
		infrastructure: backfilledEmbeddedIdentity(
			infraManifest.Name,
			presets.ListEmbeddedInfrastructuresPresets(),
			func(name string) (string, error) {
				m, err := presets.ReadInfrastructureManifest(name)
				if err != nil {
					return "", err
				}

				return m.Name, nil
			},
		),
		installation: backfilledEmbeddedIdentity(
			installManifest.Name,
			presets.ListEmbeddedInstallationsPresets(),
			func(name string) (string, error) {
				m, err := presets.ReadInstallManifest(name)
				if err != nil {
					return "", err
				}

				return m.Name, nil
			},
		),
	}

	state.InfrastructurePresetIdentity = pair.infrastructure
	state.InstallationPresetIdentity = pair.installation
	if err := config.WriteExasolPersonalState(state, deployment); err != nil {
		return presetIdentityPair{}, err
	}

	return pair, nil
}

// backfilledEmbeddedIdentity matches a manifest's display name to an embedded
// preset name. If no embedded preset matches (for example because the original
// deployment used a path preset whose original path was not persisted), it
// records the display name itself. Any future request will then fail to match
// and produce the standard mismatch error guiding the user to destroy --remove.
func backfilledEmbeddedIdentity(
	manifestName string,
	embeddedNames []string,
	readDisplayName func(string) (string, error),
) string {
	want := strings.TrimSpace(manifestName)
	for _, name := range embeddedNames {
		display, err := readDisplayName(name)
		if err == nil && strings.TrimSpace(display) == want {
			return presetIdentityOf(PresetRef{Name: name})
		}
	}

	return presetIdentityOf(PresetRef{Name: manifestName})
}
