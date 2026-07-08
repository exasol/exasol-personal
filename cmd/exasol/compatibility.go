// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/blang/semver/v4"
	deploymentcompatibility "github.com/exasol/exasol-personal/internal/compatibility"
	"github.com/exasol/exasol-personal/internal/config"
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

// requireMinorBaselineDeploymentCompatibility declares a minimum deployment
// version at minor-release granularity, for example "2.1.7" becomes "2.1.0".
func requireMinorBaselineDeploymentCompatibility(
	cmd *cobra.Command,
	minSupportedDeploymentVersion string,
) {
	minSupported, err := normalizeVersionToMinor(minSupportedDeploymentVersion)
	if err != nil {
		// Do not panic here: this helper is used when defining commands.
		// If the version is invalid, keep it as-is so the compatibility enforcement
		// returns a structured InvalidVersionError at runtime.
		requireDeploymentCompatibility(cmd, minSupportedDeploymentVersion)
		return
	}

	requireDeploymentCompatibility(cmd, minSupported)
}

// requireDefaultDeploymentCompatibility declares the normal command contract:
// commands are compatible with any deployment version unless they opt into a
// higher, named minimum because older deployment directories are unsafe for
// that command.
func requireDefaultDeploymentCompatibility(cmd *cobra.Command) {
	requireDeploymentCompatibility(cmd, minSupportedDeploymentVersionBaseline)
}

func normalizeVersionToMinor(raw string) (string, error) {
	ver, err := semver.Parse(raw)
	if err != nil {
		return "", err
	}

	// Minor-baseline requirements keep major/minor and normalize patch to 0.
	ver.Patch = 0

	// Keep normalization consistent with internal compatibility logic:
	// comparisons ignore prerelease/build metadata.
	ver.Pre = nil
	ver.Build = nil

	return ver.String(), nil
}

func requireDeploymentCompatibility(cmd *cobra.Command, minSupportedDeploymentVersion string) {
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
func enforceDeploymentDirectoryCompatibility(
	cmd *cobra.Command,
	deployment config.DeploymentDir,
) error {
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
		deployment,
		CurrentLauncherVersion,
		req,
		initReq,
	)
}
