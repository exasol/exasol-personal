// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/blang/semver/v4"
	deploymentcompatibility "github.com/exasol/exasol-personal/internal/compatibility"
	"github.com/spf13/cobra"
)

const (
	annotationRequiresDeploymentCompatibility = "exasol.requiresDeploymentCompatibility"
	annotationMinSupportedDeploymentVersion   = "exasol.minSupportedDeploymentVersion"
	annotationRequiresInitializedDeployment   = "exasol.requiresInitializedDeploymentDir"

	annotationEnabledValue = "true"

	errMissingMinSupportedFmt = "internal error: command %q requires deployment" +
		"compatibility but is missing annotation %q"
)

func requireMinorVersionCompatibility(
	cmd *cobra.Command,
	minSupportedDeploymentMinorVersion string,
) {
	minSupported, err := normalizeVersionToMinor(minSupportedDeploymentMinorVersion)
	if err != nil {
		// Do not panic here: this helper is used when defining commands.
		// If the version is invalid, keep it as-is so the compatibility enforcement
		// returns a structured InvalidVersionError at runtime.
		requireVersionCompatibility(cmd, minSupportedDeploymentMinorVersion)
		return
	}

	requireVersionCompatibility(cmd, minSupported)
}

func normalizeVersionToMinor(raw string) (string, error) {
	ver, err := semver.Parse(raw)
	if err != nil {
		return "", err
	}

	// Compatibility requirements are expressed at minor granularity:
	// keep major/minor and normalize patch to 0.
	ver.Patch = 0

	// Keep normalization consistent with internal compatibility logic:
	// comparisons ignore prerelease/build metadata.
	ver.Pre = nil
	ver.Build = nil

	return ver.String(), nil
}

func requireVersionCompatibility(cmd *cobra.Command, minSupportedDeploymentVersion string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationRequiresDeploymentCompatibility] = annotationEnabledValue
	cmd.Annotations[annotationMinSupportedDeploymentVersion] = minSupportedDeploymentVersion
}

// requireInitializedDeploymentDir declares that a command expects an existing,
// initialized deployment directory.
//
// Commands like init/install that can create the deployment directory should not
// set this.
func requireInitializedDeploymentDir(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationRequiresInitializedDeployment] = annotationEnabledValue
}

func deploymentCompatibilityIsRequired(cmd *cobra.Command) bool {
	v, ok := cmd.Annotations[annotationRequiresDeploymentCompatibility]
	return ok && v == annotationEnabledValue
}

func minSupportedDeploymentVersionFromAnnotations(cmd *cobra.Command) string {
	return cmd.Annotations[annotationMinSupportedDeploymentVersion]
}

func deploymentDirMustBeInitialized(cmd *cobra.Command) bool {
	v, ok := cmd.Annotations[annotationRequiresInitializedDeployment]
	return ok && v == annotationEnabledValue
}

// enforceDeploymentDirectoryCompatibility is a thin orchestration wrapper.
//
// It interprets command annotations (cmd-layer concern) and delegates the actual
// compatibility enforcement (including logging) to internal packages.
func enforceDeploymentDirectoryCompatibility(cmd *cobra.Command, deploymentDir string) error {
	if !deploymentCompatibilityIsRequired(cmd) {
		return nil
	}

	minSupported := minSupportedDeploymentVersionFromAnnotations(cmd)
	if minSupported == "" {
		return fmt.Errorf(
			errMissingMinSupportedFmt,
			cmd.Name(),
			annotationMinSupportedDeploymentVersion,
		)
	}

	req := deploymentcompatibility.Requirement{
		CommandName:                   cmd.Name(),
		MinSupportedDeploymentVersion: minSupported,
	}

	initReq := deploymentcompatibility.DeploymentDirMayBeUninitialized
	if deploymentDirMustBeInitialized(cmd) {
		initReq = deploymentcompatibility.DeploymentDirMustBeInitialized
	}

	return deploymentcompatibility.EnforceDeploymentDirectoryCompatibility(
		deploymentDir,
		CurrentLauncherVersion,
		req,
		initReq,
	)
}
