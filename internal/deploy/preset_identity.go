// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
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
	ctx context.Context,
	deployment config.DeploymentDir,
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
) error {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}
	existing, backfilled, err := resolvePresetIdentity(ctx, state, deployment)
	if err != nil {
		return err
	}
	if backfilled {
		if err := config.WriteExasolPersonalState(state, deployment); err != nil {
			return err
		}
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

// PresetIdentityDisplay holds display-ready preset names for a deployment.
type PresetIdentityDisplay struct {
	Infrastructure string
	Installation   string
}

// ResolveDeploymentPresetIdentity returns the display-ready infrastructure and
// installation preset names for an initialized deployment, deriving them from
// extracted manifests when the persisted state predates preset-identity
// tracking. Unlike EnsureDeploymentPresetIdentityMatches, it never writes to
// the deployment's state file: callers that only need to display the preset
// identity (such as `exasol deployments list`) must not carry a hidden write
// side effect.
func ResolveDeploymentPresetIdentity(
	ctx context.Context,
	deployment config.DeploymentDir,
) (PresetIdentityDisplay, error) {
	state, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return PresetIdentityDisplay{}, err
	}

	pair, _, err := resolvePresetIdentity(ctx, state, deployment)
	if err != nil {
		return PresetIdentityDisplay{}, err
	}

	return PresetIdentityDisplay{
		Infrastructure: presetIdentityDisplay(pair.infrastructure),
		Installation:   presetIdentityDisplay(pair.installation),
	}, nil
}

// resolvePresetIdentity returns the persisted preset identity pair. For
// deployments initialized by older launcher versions that lack the identity
// fields, it derives one from extracted manifests instead, and reports that
// via the second return value so the caller can decide whether to persist it.
// resolvePresetIdentity itself never writes to the deployment's state file.
func resolvePresetIdentity(
	ctx context.Context,
	state *config.ExasolPersonalState,
	deployment config.DeploymentDir,
) (presetIdentityPair, bool, error) {
	if state.InfrastructurePresetIdentity != "" && state.InstallationPresetIdentity != "" {
		return presetIdentityPair{
			infrastructure: state.InfrastructurePresetIdentity,
			installation:   state.InstallationPresetIdentity,
		}, false, nil
	}

	infraManifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return presetIdentityPair{}, false, fmt.Errorf("backfill preset identity: %w", err)
	}
	installManifest, err := config.ReadInstallManifest(deployment)
	if err != nil {
		return presetIdentityPair{}, false, fmt.Errorf("backfill preset identity: %w", err)
	}

	pair := presetIdentityPair{
		infrastructure: backfilledEmbeddedIdentity(
			infraManifest.Name,
			presets.ListEmbeddedPresets(ctx, presets.Infrastructure),
			func(name string) (string, error) {
				m, err := presets.ReadInfrastructureManifest(ctx, name)
				if err != nil {
					return "", err
				}

				return m.Name, nil
			},
		),
		installation: backfilledEmbeddedIdentity(
			installManifest.Name,
			presets.ListEmbeddedPresets(ctx, presets.Installation),
			func(name string) (string, error) {
				m, err := presets.ReadInstallManifest(ctx, name)
				if err != nil {
					return "", err
				}

				return m.Name, nil
			},
		),
	}

	state.InfrastructurePresetIdentity = pair.infrastructure
	state.InstallationPresetIdentity = pair.installation

	return pair, true, nil
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
