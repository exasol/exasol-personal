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
// which the deployment may already have cloud resources. It is used both when
// the deployment is live (running, stopped) and when a previous deploy or
// destroy was interrupted or failed: in those cases the cloud may still hold
// resources whose configuration the user must not silently change.
const configureNotAllowedHint = "the deployment may already have cloud resources;\n" +
	"run `exasol destroy` (or `exasol remove` if you have confirmed the cloud resources are " +
	"gone) before changing configuration, then run `exasol config set` and `exasol deploy`"

const reservedInfrastructureConfigValueCount = 5

// WorkflowStatePermitsConfigure rejects configuration changes whenever the
// deployment may already have cloud resources. Only freshly-initialized
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
	infraVars map[string]string,
	installVars map[string]string,
) error {
	infrastructureValues := make(
		map[string]string,
		len(infraVars)+reservedInfrastructureConfigValueCount,
	)
	for key, value := range infraVars {
		infrastructureValues[key] = value
	}
	installationValues := make(map[string]string, len(installVars))
	for key, value := range installVars {
		installationValues[key] = value
	}

	infraManifest, installManifest, err := readExtractedManifests(deployment)
	if err != nil {
		return err
	}

	deploymentId := exasolState.DeploymentId
	if deploymentId == "" {
		return errors.New("deployment state is missing deployment id")
	}
	clusterIdentity := exasolState.ClusterIdentity
	if clusterIdentity == "" {
		return errors.New("deployment state is missing cluster identity")
	}

	artifactDir := deployment.RelativeInfrastructureArtifactDir()
	infrastructureValues["infrastructure_artifact_dir"] = artifactDir
	infrastructureValues["installation_preset_dir"] = deployment.RelativeInstallationPresetDir()
	infrastructureValues["deployment_id"] = deploymentId
	infrastructureValues["cluster_identity"] = clusterIdentity
	infrastructureValues["deployment_created_at"] = time.Now().UTC().Format(time.RFC3339)

	backend, err := resolveBackendForManifest(infraManifest)
	if err != nil {
		return err
	}
	if err := backend.Configure(
		ctx,
		deployment,
		infraManifest,
		rawInfrastructureConfigValues(infrastructureValues),
	); err != nil {
		return err
	}

	return writeInstallationVariablesFile(
		deployment.InstallationDir(),
		installManifest.Variables,
		clusterIdentity,
		deploymentId,
		GetVersionCheckURL(),
		installationValues,
	)
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
