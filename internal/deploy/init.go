// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/util"
)

const (
	baseURL        = "https://www.exasol.com/terms-and-conditions/"
	eulaURI        = "#h-exasol-personal-end-user-license-agreement"
	eulaURL        = baseURL + eulaURI
	EulaNoticeText = `For your reference:
By using the Exasol Personal launcher, you accept its End User License Agreement (EULA):
` + eulaURL + `

A copy of the EULA is also included as 'eula.txt' in this directory.

`
)

// ResolveInfrastructureInfo validates the infrastructure preset name and returns its info.
func ResolveInfrastructureInfo(infrastructureName string) (*InfrastructureInfo, error) {
	// Proactively validate against known infrastructures to produce a clearer error.
	known := presets.ListEmbeddedInfrastructuresPresets()
	found := false
	for _, k := range known {
		if k == infrastructureName {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown infrastructure preset %q", infrastructureName)
	}

	info, err := GetInfrastructureInfo(infrastructureName)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get infrastructure info for %q: %w",
			infrastructureName,
			err,
		)
	}

	return info, nil
}

var (
	ErrUnknownVariable             = errors.New("unknown variable")
	ErrDeploymentDirectoryNotEmpty = errors.New("deployment directory is not empty")
	ErrDeploymentPresetMismatch    = errors.New(
		"deployment directory is initialized with different presets",
	)
)

// InitDeployment initializes a new deployment directory by extracting presets and
// creating the variables file based on the infrastructure manifest.
//
//nolint:revive // versionCheckEnabled is a user-controlled flag, not internal control coupling
func InitDeployment(
	ctx context.Context,
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
	infraVars map[string]string,
	installVars map[string]string,
	deployment config.DeploymentDir,
	versionCheckEnabled bool,
	currentVersion string,
) error {
	// Do an initial update version check if permitted
	if versionCheckEnabled {
		_, _, _ = CheckLatestVersionUpdate(ctx, currentVersion, deployment)
	}

	// Proactively validate the preset selection to produce friendly errors.
	slog.Info("validating presets")
	if err := ValidatePresetSelection(infrastructurePreset, installationPreset); err != nil {
		return err
	}
	infrastructureManifest, err := readInfrastructureManifestFromPreset(infrastructurePreset)
	if err != nil {
		return fmt.Errorf(
			"failed to load infrastructure preset %q: %w",
			presetLabel(infrastructurePreset),
			err,
		)
	}
	backend, err := newDeploymentBackend(deployment, infrastructureManifest)
	if err != nil {
		return err
	}
	if err := backend.ValidateEnvironment(); err != nil {
		return err
	}
	if err := validateLocalInitMemory(ctx, infrastructureManifest, infraVars); err != nil {
		return err
	}

	// Init only creates fresh deployment state. Existing deployment orchestration
	// belongs to the command layer.
	initialized, err := config.HasExasolPersonalStateFile(deployment)
	if err != nil {
		slog.Error("failed to check deployment directory initialization")
		return err
	}
	if initialized {
		return ErrDeploymentDirectoryNotEmpty
	}

	// Make sure the directory exists and is empty
	if err = util.EnsureDir(deployment.Root()); err != nil {
		return err
	}

	if err = ensureDirectoryIsEmpty(deployment); err != nil {
		return err
	}

	// Lock the deployment directory with exclusive access
	return withDeploymentExclusiveLock(ctx, deployment,
		func(deployment config.DeploymentDir) error {
			deploymentId, err := GenerateDeploymentId()
			if err != nil {
				return fmt.Errorf("failed to generate deployment id: %w", err)
			}
			clusterIdentity := ComputeClusterIdentity(
				deploymentId,
				infrastructurePreset,
				installationPreset,
			)

			// Copy the presets into the deployment directory
			err = extractPresets(infrastructurePreset, installationPreset, deployment)
			if err != nil {
				return err
			}

			slog.Debug("Initializing deployment state")
			exasolState := newInitializedState(
				versionCheckEnabled,
				currentVersion,
				deploymentId,
				clusterIdentity,
				time.Now().UTC(),
				infrastructurePreset,
				installationPreset,
			)
			infraManifest, _, err := readExtractedManifests(deployment)
			if err != nil {
				return err
			}
			backend, err := newDeploymentBackend(deployment, infraManifest)
			if err != nil {
				return err
			}
			if err := backend.SetupWorkspace(ctx); err != nil {
				return err
			}
			if err := writeDeploymentConfiguration(
				ctx,
				deployment,
				exasolState,
				newDeploymentConfigurationFromRaw(infraVars, installVars),
			); err != nil {
				return err
			}

			err = exasolState.SetWorkflowStateAndWrite(
				&config.WorkflowStateInitialized{},
				deployment,
			)
			if err != nil {
				return err
			}
			if err := config.WriteDeploymentVersionMarker(deployment, currentVersion); err != nil {
				return err
			}

			slog.Info(
				"successfully initialized deployment",
				"infrastructure",
				presetLabel(infrastructurePreset),
				"installation",
				presetLabel(installationPreset),
			)

			return nil
		})
}

// extractPresets writes infrastructure, installation,
// and shared assets into the deployment directory.
func extractPresets(
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
	deployment config.DeploymentDir,
) error {
	slog.Info("extracting preset files",
		"infrastructure", infrastructurePreset,
		"installation", installationPreset)

	infrastructureDir := deployment.InfrastructureDir()
	installationDir := deployment.InstallationDir()

	// Write shared assets
	slog.Debug("writing shared files to deployment directory", "dest", ".")
	err := presets.WriteSharedDir(deployment.Root())
	if err != nil {
		slog.Error(
			"Failed to write shared assets",
			"err", err,
			"dir", util.AbsPathNoFail(deployment.Root()),
		)

		return err
	}

	// Write infrastructure preset
	slog.Debug("writing infrastructure preset to deployment directory", "path", infrastructureDir)
	err = presets.ExtractPreset(
		infrastructurePreset,
		infrastructureDir,
		presets.WriteInfrastructureDir,
	)
	if err != nil {
		slog.Error(
			"Failed to write infrastructure preset",
			"err", err,
			"infrastructure", presetLabel(infrastructurePreset),
			"dir", util.AbsPathNoFail(infrastructureDir),
		)

		return err
	}

	// Write installation preset into installation directory
	slog.Debug("writing installation preset to deployment directory", "path", installationDir)
	err = presets.ExtractPreset(installationPreset, installationDir, presets.WriteInstallDir)
	if err != nil {
		slog.Error(
			"Failed to write installation preset",
			"err", err,
			"installation", presetLabel(installationPreset),
			"dir", util.AbsPathNoFail(installationDir),
		)

		return err
	}

	return nil
}

func ensureDirectoryIsEmpty(deployment config.DeploymentDir) error {
	// When init is called, the deployment dir must be empty.
	slog.Debug("testing if deployment directory is empty")
	entries, err := os.ReadDir(deployment.Root())
	if err != nil {
		return err
	}

	if len(entries) != 0 {
		badFile := filepath.Join(deployment.Root(), entries[0].Name())
		slog.Error(ErrDeploymentDirectoryNotEmpty.Error(), "file", util.AbsPathNoFail(badFile))

		return fmt.Errorf("%w: file: \"%s\"", ErrDeploymentDirectoryNotEmpty, badFile)
	}

	return nil
}

//nolint:revive // versionCheckEnabled chooses persisted state shape for user configuration.
func newInitializedState(
	versionCheckEnabled bool,
	deploymentVersion string,
	deploymentId string,
	clusterIdentity string,
	createdAt time.Time,
	infrastructurePreset PresetRef,
	installationPreset PresetRef,
) *config.ExasolPersonalState {
	lastVersionCheck := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	if versionCheckEnabled {
		lastVersionCheck = time.Now()
	}

	return &config.ExasolPersonalState{
		DeploymentId:                 deploymentId,
		ClusterIdentity:              clusterIdentity,
		CreatedAt:                    createdAt.UTC(),
		DeploymentVersion:            deploymentVersion,
		VersionCheckEnabled:          versionCheckEnabled,
		LastVersionCheck:             lastVersionCheck,
		InfrastructurePresetIdentity: presetIdentityOf(infrastructurePreset),
		InstallationPresetIdentity:   presetIdentityOf(installationPreset),
	}
}

func validateInfrastructurePreset(infrastructurePreset PresetRef) error {
	if infrastructurePreset.IsPath() {
		return validatePresetDir(infrastructurePreset.Path, "infrastructure.yaml")
	}

	known := presets.ListEmbeddedInfrastructuresPresets()
	for _, k := range known {
		if k == infrastructurePreset.Name {
			return nil
		}
	}

	return fmt.Errorf(
		"unknown infrastructure preset %q; available: %s",
		infrastructurePreset.Name,
		strings.Join(known, ", "),
	)
}

func validateInstallationPreset(installationPreset PresetRef) error {
	if installationPreset.IsPath() {
		return validatePresetDir(installationPreset.Path, "installation.yaml")
	}

	known := presets.ListEmbeddedInstallationsPresets()
	for _, k := range known {
		if k == installationPreset.Name {
			return nil
		}
	}

	return fmt.Errorf(
		"unknown installation preset %q; available: %s",
		installationPreset.Name,
		strings.Join(known, ", "),
	)
}

func validatePresetDir(dir, requiredFile string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("preset path %q does not exist", dir)
		}

		return fmt.Errorf("failed to access preset path %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("preset path %q is not a directory", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, requiredFile)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("preset path %q is missing required file %q", dir, requiredFile)
		}

		return fmt.Errorf(
			"failed to validate preset path %q (required file %q): %w",
			dir,
			requiredFile,
			err,
		)
	}

	return nil
}

func presetLabel(p PresetRef) string {
	if p.IsPath() {
		return p.Path
	}

	return p.Name
}
