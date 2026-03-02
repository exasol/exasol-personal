// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploymentcompatibility

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// Reason describes why a compatibility check failed.
type Reason string

const (
	ReasonDeploymentNewerThanLauncher Reason = "deployment_newer_than_launcher"
	ReasonDeploymentTooOld            Reason = "deployment_too_old_for_command"
)

// Action describes what the user needs to do to resolve an incompatibility.
type Action string

const (
	ActionUpgradeLauncher       Action = "upgrade_launcher"
	ActionUseCompatibleLauncher Action = "use_compatible_launcher"
)

// IncompatibleError is returned when a deployment directory is not compatible
// with the currently running launcher for a given command.
//
// This error is designed to support actionable, user-facing messaging in the
// cmd layer without printing from internal packages.
type IncompatibleError struct {
	DeploymentVersion semver.Version
	LauncherVersion   semver.Version
	MinSupported      semver.Version
	CommandName       string
	Reason            Reason
	RequiredAction    Action
}

func (e *IncompatibleError) Error() string {
	cmd := e.CommandName
	if cmd == "" {
		cmd = "(unspecified command)"
	}

	switch e.Reason {
	case ReasonDeploymentNewerThanLauncher:
		return fmt.Sprintf(
			"deployment directory is incompatible with this launcher:\n"+
				"Deployment version %s is newer than launcher version %s (command %s)\n"+
				"Required action: upgrade the launcher to version >= %s",
			e.DeploymentVersion,
			e.LauncherVersion,
			cmd,
			e.DeploymentVersion,
		)
	case ReasonDeploymentTooOld:
		return fmt.Sprintf(
			"deployment directory is incompatible with this command:\n"+
				"Deployment version %s is older than the minimum supported "+
				"deployment version %s for launcher version %s (command %s)\n"+
				"Required action: use a launcher version that supports this deployment",
			e.DeploymentVersion,
			e.MinSupported,
			e.LauncherVersion,
			cmd,
		)
	default:
		return fmt.Sprintf(
			"deployment directory is incompatible (command %s); required action: %s",
			cmd,
			e.RequiredAction,
		)
	}
}

// InvalidVersionError is returned when an input version string is missing or
// not a valid semantic version.
type InvalidVersionError struct {
	Label string
	Raw   string
	Cause error
}

func (e *InvalidVersionError) Error() string {
	if e.Raw == "" {
		return fmt.Sprintf("invalid %s: missing", e.Label)
	}

	return fmt.Sprintf("invalid %s %q: %v", e.Label, e.Raw, e.Cause)
}

func (e *InvalidVersionError) Unwrap() error { return e.Cause }
