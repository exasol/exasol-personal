// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploymentcompatibility

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	legacyWorkflowStateFileName = ".workflowState.json"

	exampleLegacyLauncherVersion = "1.0.0"

	errLegacyDeploymentLayoutFmt = "deployment directory appears to be from an older version " +
		"(found %q but missing %q).\n" +
		"This launcher cannot operate on it. " +
		"Use a compatible older launcher (e.g. %s) or recreate the deployment directory"
	errDeploymentNotInitializedFmt = "deployment directory is not initialized " +
		"(missing %q in %q).\n" +
		"Run 'exasol init' or 'exasol install' in that directory, " +
		"or pass --deployment-dir pointing to an existing deployment directory"
	errDeploymentVersionMarkerMissingFmt = "deployment directory is missing deployment " +
		"version marker %q in %q.\n" +
		"Recreate the deployment directory with this launcher " +
		"(run 'exasol init' or 'exasol install') " +
		"or ensure --deployment-dir points to the correct directory"
	errDeploymentVersionMarkerEmptyFmt = "deployment version marker %q in %q is empty"
)

// DeploymentDirInitializationRequirement describes whether a command expects an
// already-initialized deployment directory.
type DeploymentDirInitializationRequirement int

const (
	DeploymentDirMayBeUninitialized DeploymentDirInitializationRequirement = iota
	DeploymentDirMustBeInitialized
)

// EnforceDeploymentDirectoryCompatibility validates whether a command is allowed to
// operate on the deployment directory.
//
// Design decision: this function performs logging internally so the cmd layer can
// stay focused on orchestration and terminal output.
//
// If initReq is DeploymentDirMayBeUninitialized, the function allows truly
// empty/uninitialized directories (for commands like init/install), but still
// fails on known legacy layouts.
func EnforceDeploymentDirectoryCompatibility(
	deploymentDir string,
	launcherVersion string,
	req Requirement,
	initReq DeploymentDirInitializationRequirement,
) error {
	initialized, err := config.IsDirectoryContainingStateFile(deploymentDir)
	if err != nil {
		return err
	}
	if !initialized {
		return handleUninitializedDeploymentDir(req.CommandName, deploymentDir, initReq)
	}

	deploymentVersion, err := readDeploymentVersionMarker(deploymentDir)
	if err != nil {
		return err
	}

	result := Check(deploymentVersion, launcherVersion, req)
	if result.Allowed {
		return nil
	}

	logCompatibilityFailure(req.CommandName, result.Err)

	return result.Err
}

func handleUninitializedDeploymentDir(
	commandName string,
	deploymentDir string,
	initReq DeploymentDirInitializationRequirement,
) error {
	legacy, err := legacyWorkflowStateExists(deploymentDir)
	if err != nil {
		return err
	}
	if legacy {
		err := fmt.Errorf(
			errLegacyDeploymentLayoutFmt,
			legacyWorkflowStateFileName,
			config.ExasolPersonalStateFileName,
			exampleLegacyLauncherVersion,
		)
		slog.Warn(
			"legacy deployment directory detected",
			"command", commandName,
			"deployment_dir", deploymentDir,
		)

		return err
	}

	if initReq == DeploymentDirMayBeUninitialized {
		return nil
	}

	err = fmt.Errorf(
		errDeploymentNotInitializedFmt,
		config.ExasolPersonalStateFileName,
		deploymentDir,
	)
	slog.Error(
		"deployment directory is not initialized",
		"command", commandName,
		"deployment_dir", deploymentDir,
	)

	return err
}

func legacyWorkflowStateExists(deploymentDir string) (bool, error) {
	_, err := os.Stat(filepath.Join(deploymentDir, legacyWorkflowStateFileName))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, err
}

func readDeploymentVersionMarker(deploymentDir string) (string, error) {
	ver, ok, err := config.ReadDeploymentVersionMarker(deploymentDir)
	if err != nil {
		return "", err
	}
	if !ok {
		err := fmt.Errorf(
			errDeploymentVersionMarkerMissingFmt,
			config.DeploymentVersionMarkerFileName,
			deploymentDir,
		)
		slog.Error(
			"deployment version marker missing",
			"deployment_dir", deploymentDir,
			"marker", config.DeploymentVersionMarkerFileName,
			"error", err.Error(),
		)

		return "", err
	}
	if ver == "" {
		err := fmt.Errorf(
			errDeploymentVersionMarkerEmptyFmt,
			config.DeploymentVersionMarkerFileName,
			deploymentDir,
		)
		slog.Error(
			"deployment version marker empty",
			"deployment_dir", deploymentDir,
			"marker", config.DeploymentVersionMarkerFileName,
			"error", err.Error(),
		)

		return "", err
	}

	return ver, nil
}

func logCompatibilityFailure(commandName string, err error) {
	// Structured logging is useful for automation/diagnostics while keeping cmd output
	// as just the returned error message.
	var inc *IncompatibleError
	if errors.As(err, &inc) {
		slog.Error(
			"deployment directory compatibility check failed",
			"command", inc.CommandName,
			"deployment_version", inc.DeploymentVersion.String(),
			"launcher_version", inc.LauncherVersion.String(),
			"min_supported_deployment_version", inc.MinSupported.String(),
			"reason", string(inc.Reason),
			"required_action", string(inc.RequiredAction),
			"error", inc.Error(),
		)

		return
	}
	if err != nil {
		slog.Error(
			"deployment directory compatibility check failed",
			"command", commandName,
			"error", err,
		)
	}
}
