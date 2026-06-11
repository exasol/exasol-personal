// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
)

var ErrConfigureNotAllowed = errors.New("deployment cannot be configured in its current state")

// configureNotAllowedHint guides the user out of any non-initialized state in
// which the deployment may already have resources. It is used both when
// the deployment is live (running, stopped) and when a previous deploy or
// destroy was interrupted or failed: in those cases the target infrastructure may still hold
// resources whose configuration the user must not silently change.
const configureNotAllowedHint = "the deployment may already have resources;\n" +
	"run `exasol destroy` (or `exasol remove` if you have confirmed the resources are " +
	"gone) before changing configuration, then run `exasol config set` and `exasol deploy`"

type DeploymentID string

type ClusterIdentity string

type RelativeDeploymentPath string

type DeploymentMetadata struct {
	ID              DeploymentID
	ClusterIdentity ClusterIdentity
	CreatedAt       time.Time
}

type DeploymentLayout struct {
	InfrastructureArtifactDir RelativeDeploymentPath
	InstallationPresetDir     RelativeDeploymentPath
}

// WorkflowStatePermitsConfigure rejects configuration changes whenever the
// deployment may already have resources. Only freshly-initialized
// deployments may be configured; any later state (running, stopped, deployment
// failed, interrupted during deploy or destroy, or an operation in progress)
// requires the user to run `exasol destroy` (or `exasol remove`) first so that
// configuration changes cannot diverge from the actual deployed cloud state.
func WorkflowStatePermitsConfigure(exasolState *config.ExasolPersonalState) error {
	workflowState, err := exasolState.GetWorkflowState()
	if err != nil {
		return err
	}

	if _, ok := workflowState.(*config.WorkflowStateInitialized); ok {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrConfigureNotAllowed, configureNotAllowedHint)
}

func writeDeploymentConfiguration(
	ctx context.Context,
	deployment config.DeploymentDir,
	exasolState *config.ExasolPersonalState,
	configuration DeploymentConfiguration,
) error {
	infraManifest, installManifest, err := readExtractedManifests(deployment)
	if err != nil {
		return err
	}
	backend, err := newDeploymentBackend(deployment, infraManifest)
	if err != nil {
		return err
	}

	metadata, err := resolveDeploymentMetadata(exasolState)
	if err != nil {
		return err
	}
	layout := DeploymentLayout{
		InfrastructureArtifactDir: RelativeDeploymentPath(
			deployment.RelativeInfrastructureArtifactDir(),
		),
		InstallationPresetDir: RelativeDeploymentPath(deployment.RelativeInstallationPresetDir()),
	}
	if err := backend.Configure(
		ctx,
		configValuesRawMap(configuration.Infrastructure.Options),
		metadata,
		layout,
	); err != nil {
		return err
	}

	return writeInstallationVariablesFile(
		deployment.InstallationDir(),
		installManifest.Variables,
		string(metadata.ClusterIdentity),
		string(metadata.ID),
		GetVersionCheckURL(),
		configValuesRawMap(configuration.Installation.Options),
	)
}

func resolveDeploymentMetadata(
	exasolState *config.ExasolPersonalState,
) (DeploymentMetadata, error) {
	deploymentId := exasolState.DeploymentId
	if deploymentId == "" {
		return DeploymentMetadata{}, errors.New("deployment state is missing deployment id")
	}
	clusterIdentity := exasolState.ClusterIdentity
	if clusterIdentity == "" {
		return DeploymentMetadata{}, errors.New("deployment state is missing cluster identity")
	}
	createdAt := exasolState.CreatedAt
	if createdAt.IsZero() {
		// Older deployment-state files predate the launcher-owned
		// createdAt field; fall back to "now" to keep behaviour stable.
		createdAt = time.Now().UTC()
	}

	return DeploymentMetadata{
		ID:              DeploymentID(deploymentId),
		ClusterIdentity: ClusterIdentity(clusterIdentity),
		CreatedAt:       createdAt.UTC(),
	}, nil
}

func readExtractedManifests(
	deployment config.DeploymentDir,
) (*presets.InfrastructureManifest, *presets.InstallManifest, error) {
	slog.Debug("loading preset manifests")
	infraManifest, err := presets.ReadInfrastructureManifestFromDir(deployment.InfrastructureDir())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read extracted infrastructure manifest: %w", err)
	}
	installManifest, err := presets.ReadInstallManifestFromDir(deployment.InstallationDir())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read extracted installation manifest: %w", err)
	}

	return infraManifest, installManifest, nil
}
