// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

// This file centralizes deployment compatibility version constants.
//
// Why this exists:
// - Many commands co-evolve in what deployment versions they can safely operate on.
// - We want to avoid sprinkling raw semver literals (e.g. "0.0.0") across the cmd layer.
// - When we introduce a breaking change in deployment directory semantics, we add a
//   new constant here (named after the change) and switch affected commands to it.
//
// Rules of thumb for maintainers:
// - Bump minimum supported deployment versions only when a command would be unsafe
//   or misleading on older deployments.
// - Prefer introducing a new constant with a comment explaining the breaking change
//   (what changed and why older deployments are incompatible).
// - Do not include prerelease/build suffixes in these constants. Compatibility
//   comparisons ignore suffixes like "-rc1" and compare only the base version.

const (
	// minSupportedDeploymentVersionBaseline is the default minimum supported deployment version.
	//
	// It is intentionally set to "0.0.0" so that development builds (which commonly
	// report version "0.0.0") can create and operate on deployments without tripping
	// compatibility checks.
	//
	// When a real breaking change is introduced, define a new constant below and use
	// it for the affected commands.
	minSupportedDeploymentVersionBaseline = "0.0.0"

	// Example for the next breaking change:
	//	// minSupportedDeploymentVersion_<featureName> is the first deployment version
	//	// that includes <featureName> changes (explain the incompatibility here).
	//	minSupportedDeploymentVersion_<featureName> = "X.Y.Z"

	// NOTE: use this once it is clear in which version this change was released:
	// The deployment id now generated and owner by the launcher instead of the infra preset
	// The deployment id is also removed from the file artifacts like `secrets.json`
	// which are written as part of the contract between launcher and infra preset.
	// minSupported_deploymentIdByLauncher = "1.3.0" ??
)
