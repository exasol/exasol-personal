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
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/presets"
	"github.com/exasol/exasol-personal/internal/tofu"
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
	deploymentDir string,
	versionCheckEnabled bool,
	currentVersion string,
) error {
	// Do an initial update version check if permitted
	if versionCheckEnabled {
		_, _, _ = CheckLatestVersionUpdate(ctx, currentVersion, deploymentDir)
	}

	// Proactively validate the preset selection to produce friendly errors.
	slog.Info("validating presets")
	if err := validateInfrastructurePreset(infrastructurePreset); err != nil {
		return err
	}
	if err := validateInstallationPreset(installationPreset); err != nil {
		return err
	}

	// If the directory is already an initialized deployment directory,
	// we skip everything and regard this as a success
	initialized, err := config.IsDirectoryContainingStateFile(deploymentDir)
	if err != nil {
		slog.Error("failed to check deployment directory initialization")
		return err
	} else if initialized {
		slog.Info("deployment directory is already initialized")
		return nil
	}

	// Make sure the directory is empty
	if err = ensureDirectoryIsEmpty(deploymentDir); err != nil {
		return err
	}

	// Lock the deployment directory with exclusive access
	return withDeploymentExclusiveLock(ctx, deploymentDir,
		func(deploymentDir string) error {
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
			err = extractPresets(infrastructurePreset, installationPreset, deploymentDir)
			if err != nil {
				return err
			}

			// Load manifests from the extracted presets (the deployment directory is the source of truth).
			slog.Debug("loading preset manifests")
			infraDir := filepath.Join(deploymentDir, config.InfrastructureFilesDirectory)
			infraManifest, err := presets.ReadInfrastructureManifestFromDir(infraDir)
			if err != nil {
				return fmt.Errorf("failed to read extracted infrastructure manifest: %w", err)
			}
			installDir := filepath.Join(deploymentDir, config.InstallationFilesDirectory)
			installManifest, err := presets.ReadInstallManifestFromDir(installDir)
			if err != nil {
				return fmt.Errorf("failed to read extracted installation manifest: %w", err)
			}

			// These values should always be part of the infra vars per contract
			// It tell the infrastructure preset where to write deployment artifacts fot the launcher
			infraVars["infrastructure_artifact_dir"] = deploymentDir
			infraVars["installation_preset_dir"] = installDir
			// Launcher-governed identity values.
			infraVars["deployment_id"] = deploymentId
			infraVars["cluster_identity"] = clusterIdentity
			infraVars["deployment_created_at"] = time.Now().UTC().Format(time.RFC3339)

			// If tofu is configured for the infrastructure, perform tofu-specific initialization.
			if infraManifest.Tofu != nil {
				tofuCfg := tofu.NewTofuConfigFromDeployment(deploymentDir, *infraManifest.Tofu)
				slog.Info("preparing tofu workspace", "workdir", tofuCfg.WorkDir())
				if err := tofu.Prepare(tofuCfg, infraVars); err != nil {
					return err
				}
			}

			if err := writeInstallationVariablesFile(
				installDir,
				installManifest.Variables,
				clusterIdentity,
				deploymentId,
				installVars,
			); err != nil {
				return err
			}

			slog.Debug("Initializing deployment state")
			if versionCheckEnabled {
				if err := writeInitializedStateWithVersionChecks(
					deploymentDir,
					currentVersion,
					deploymentId,
					clusterIdentity,
				); err != nil {
					return err
				}
			} else {
				if err := writeInitializedStateWithoutVersionChecks(
					deploymentDir,
					currentVersion,
					deploymentId,
					clusterIdentity,
				); err != nil {
					return err
				}
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
	deploymentDir string,
) error {
	slog.Info("extracting preset files",
		"infrastructure", infrastructurePreset,
		"installation", installationPreset)

	infrastructureDir := filepath.Join(deploymentDir, config.InfrastructureFilesDirectory)
	installationDir := filepath.Join(deploymentDir, config.InstallationFilesDirectory)

	// Write shared assets
	slog.Debug("writing shared files to deployment directory", "dest", ".")
	err := presets.WriteSharedDir(deploymentDir)
	if err != nil {
		slog.Error(
			"Failed to write shared assets",
			"err", err,
			"dir", util.AbsPathNoFail(deploymentDir),
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

func ensureDirectoryIsEmpty(deploymentDir string) error {
	// When init is called, the deployment dir must be empty.
	slog.Debug("testing if deployment directory is empty")
	entries, err := util.ListDir(deploymentDir, 1)
	if err != nil {
		return err
	}

	if len(entries) != 0 {
		badFile := filepath.Join(deploymentDir, entries[0])
		slog.Error(ErrDeploymentDirectoryNotEmpty.Error(), "file", util.AbsPathNoFail(badFile))

		return fmt.Errorf("%w: file: \"%s\"", ErrDeploymentDirectoryNotEmpty, badFile)
	}

	return nil
}

func writeInitializedStateWithVersionChecks(
	deploymentDir string,
	deploymentVersion string,
	deploymentId string,
	clusterIdentity string,
) error {
	exasolState := newInitializedStateWithVersionChecks(
		deploymentVersion,
		deploymentId,
		clusterIdentity,
	)
	err := exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInitialized{}, deploymentDir)
	if err != nil {
		return err
	}

	return config.WriteDeploymentVersionMarker(deploymentDir, deploymentVersion)
}

func writeInitializedStateWithoutVersionChecks(
	deploymentDir string,
	deploymentVersion string,
	deploymentId string,
	clusterIdentity string,
) error {
	exasolState := newInitializedStateWithoutVersionChecks(
		deploymentVersion,
		deploymentId,
		clusterIdentity,
	)
	err := exasolState.SetWorkflowStateAndWrite(&config.WorkflowStateInitialized{}, deploymentDir)
	if err != nil {
		return err
	}

	return config.WriteDeploymentVersionMarker(deploymentDir, deploymentVersion)
}

func newInitializedStateWithVersionChecks(
	deploymentVersion string,
	deploymentId string,
	clusterIdentity string,
) *config.ExasolPersonalState {
	return &config.ExasolPersonalState{
		DeploymentId:        deploymentId,
		ClusterIdentity:     clusterIdentity,
		DeploymentVersion:   deploymentVersion,
		VersionCheckEnabled: true,
		LastVersionCheck:    time.Now(),
	}
}

func newInitializedStateWithoutVersionChecks(
	deploymentVersion string,
	deploymentId string,
	clusterIdentity string,
) *config.ExasolPersonalState {
	return &config.ExasolPersonalState{
		DeploymentId:        deploymentId,
		ClusterIdentity:     clusterIdentity,
		DeploymentVersion:   deploymentVersion,
		VersionCheckEnabled: false,
		LastVersionCheck:    time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
}

func validateInfrastructurePreset(infrastructurePreset PresetRef) error {
	if infrastructurePreset.IsPath() {
		return validatePresetDir(infrastructurePreset.Path, "infrastructure.yaml")
	}

	for _, k := range presets.ListEmbeddedInfrastructuresPresets() {
		if k == infrastructurePreset.Name {
			return nil
		}
	}

	return fmt.Errorf("unknown infrastructure preset %q", infrastructurePreset.Name)
}

func validateInstallationPreset(installationPreset PresetRef) error {
	if installationPreset.IsPath() {
		return validatePresetDir(installationPreset.Path, "installation.yaml")
	}

	for _, k := range presets.ListEmbeddedInstallationsPresets() {
		if k == installationPreset.Name {
			return nil
		}
	}

	return fmt.Errorf("unknown installation preset %q", installationPreset.Name)
}

func validatePresetDir(dir string, requiredFile string) error {
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
