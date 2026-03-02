// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package deploymentcompatibility provides a centralized, generic compatibility
// check between a deployment directory (created by some launcher version) and
// the currently running launcher.
//
// The key safety rule is strict: if the deployment was created by a newer
// launcher version than the current binary, commands must never proceed.
package deploymentcompatibility

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// Requirement describes the minimum deployment version a command supports.
//
// The launcher version is not used as an upper bound for compatibility.
// Instead, the framework enforces a strict rule: deployment versions newer than
// the current launcher are always rejected.
//
// MinSupportedDeploymentVersion must be a valid semantic version.
//
// CommandName is optional and only used for diagnostics.
type Requirement struct {
	CommandName                   string
	MinSupportedDeploymentVersion string
}

// Result is the outcome of a compatibility check.
//
// If Allowed is false, Err describes why execution must be blocked.
type Result struct {
	Allowed bool
	Err     error
}

// Check validates whether a command is allowed to operate on a deployment
// directory.
//
// Inputs:
//   - deploymentVersion: version persisted in the deployment directory state
//   - launcherVersion: version of the currently running launcher binary
//   - req: command-specific compatibility requirement
func Check(deploymentVersion, launcherVersion string, req Requirement) Result {
	depV, err := parseRequiredVersion(deploymentVersion, "deployment version")
	if err != nil {
		return Result{Allowed: false, Err: err}
	}
	launchV, err := parseRequiredVersion(launcherVersion, "launcher version")
	if err != nil {
		return Result{Allowed: false, Err: err}
	}
	minV, err := parseRequiredVersion(
		req.MinSupportedDeploymentVersion,
		"minimum supported deployment version",
	)
	if err != nil {
		return Result{Allowed: false, Err: err}
	}

	// Strict forward-incompatibility rule: an older launcher must never operate on
	// a deployment created by a newer launcher.
	if depV.GT(launchV) {
		return Result{Allowed: false, Err: &IncompatibleError{
			DeploymentVersion: depV,
			LauncherVersion:   launchV,
			MinSupported:      minV,
			CommandName:       req.CommandName,
			Reason:            ReasonDeploymentNewerThanLauncher,
			RequiredAction:    ActionUpgradeLauncher,
		}}
	}

	if depV.LT(minV) {
		return Result{Allowed: false, Err: &IncompatibleError{
			DeploymentVersion: depV,
			LauncherVersion:   launchV,
			MinSupported:      minV,
			CommandName:       req.CommandName,
			Reason:            ReasonDeploymentTooOld,
			RequiredAction:    ActionUseCompatibleLauncher,
		}}
	}

	return Result{Allowed: true, Err: nil}
}

func parseRequiredVersion(raw string, label string) (semver.Version, error) {
	if raw == "" {
		return semver.Version{}, &InvalidVersionError{
			Label: label,
			Raw:   raw,
			Cause: fmt.Errorf("missing %s", label),
		}
	}
	ver, err := semver.Parse(raw)
	if err != nil {
		return semver.Version{}, &InvalidVersionError{Label: label, Raw: raw, Cause: err}
	}

	// Design decision: compatibility comparisons ignore prerelease and build metadata.
	//
	// We publish release candidates like "1.2.0-rc1". For compatibility decisions we
	// treat these as "1.2.0" to avoid artificial incompatibilities between a release
	// and its RCs.
	ver.Pre = nil
	ver.Build = nil

	return ver, nil
}
