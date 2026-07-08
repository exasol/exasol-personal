// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	deploymentcompatibility "github.com/exasol/exasol-personal/internal/compatibility"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/spf13/cobra"
)

func TestEnforceDeploymentDirectoryCompatibility_FailsEarlyWhenNotInitialized(t *testing.T) {
	t.Parallel()

	// Given: an empty, uninitialized deployment directory and a command that
	// requires an initialized deployment directory.
	tmp := t.TempDir()
	cmd := &cobra.Command{Use: "deploy"}
	requireDeploymentCompatibility(cmd, minSupportedDeploymentVersionBaseline)
	requireInitializedDeploymentDir(cmd)

	// When: compatibility enforcement runs.
	err := enforceDeploymentDirectoryCompatibility(cmd, config.NewDeploymentDir(tmp))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Then: it fails early with a helpful error message.
	msg := err.Error()
	if !strings.Contains(msg, "deployment directory is not initialized") {
		t.Fatalf("unexpected error message: %q", msg)
	}
	if !strings.Contains(msg, ".exasolLauncherState.json") {
		t.Fatalf("expected error to mention state file, got: %q", msg)
	}
	if !strings.Contains(msg, "exasol init") || !strings.Contains(msg, "exasol install") {
		t.Fatalf("expected error to mention init/install guidance, got: %q", msg)
	}
}

func TestEnforceDeploymentDirectoryCompatibility_HintsLegacyWorkflowStateLayout(t *testing.T) {
	t.Parallel()

	// Given: a deployment directory that matches the legacy layout used before
	// the launcher state file existed.
	tmp := t.TempDir()
	err := os.WriteFile(filepath.Join(tmp, ".workflowState.json"), []byte("{}"), 0o600)
	if err != nil {
		t.Fatalf("failed to create legacy workflow state file: %v", err)
	}
	// Given: an init-like command that may operate on uninitialized directories
	// (so it must not require an initialized deployment dir).
	cmd := &cobra.Command{Use: "some_init_like_command"}
	requireDeploymentCompatibility(cmd, minSupportedDeploymentVersionBaseline)

	// When: compatibility enforcement runs.
	err = enforceDeploymentDirectoryCompatibility(cmd, config.NewDeploymentDir(tmp))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Then: it fails with an explicit hint that the directory is from an older
	// launcher layout.
	msg := err.Error()
	if !strings.Contains(msg, "deployment directory appears to be from an older version") {
		t.Fatalf("expected legacy version hint, got: %q", msg)
	}
	if !strings.Contains(msg, ".workflowState.json") {
		t.Fatalf("expected message to mention legacy file, got: %q", msg)
	}
	if !strings.Contains(msg, "1.0.0") {
		t.Fatalf("expected message to suggest older launcher version, got: %q", msg)
	}
}

func TestRequireMinorBaselineDeploymentCompatibility_NormalizesPatchToZero(t *testing.T) {
	t.Parallel()

	// Given: a command and a semver string that includes patch, prerelease and build metadata.
	cmd := &cobra.Command{Use: "deploy"}

	// When: the command declares a minor-level minimum.
	requireMinorBaselineDeploymentCompatibility(cmd, "1.2.3-rc1+build.7")

	// Then: the stored minimum is normalized to major.minor.0 and compatibility is required.
	if !deploymentCompatibilityIsRequired(cmd) {
		t.Fatal("expected deployment compatibility to be required")
	}
	got := minSupportedDeploymentVersionFromAnnotations(cmd)
	if got != "1.2.0" {
		t.Fatalf("expected min supported deployment version to be normalized to 1.2.0, got %q", got)
	}
}

func TestRequireDefaultDeploymentCompatibilityUsesStableBaseline(t *testing.T) {
	t.Parallel()

	// Given: a command uses the default deployment compatibility declaration.
	cmd := &cobra.Command{Use: "status"}

	// When: the command declares compatibility requirements.
	requireDefaultDeploymentCompatibility(cmd)

	// Then: it uses the stable baseline instead of the current launcher version.
	if !deploymentCompatibilityIsRequired(cmd) {
		t.Fatal("expected deployment compatibility to be required")
	}
	if got := minSupportedDeploymentVersionFromAnnotations(cmd); got != "0.0.0" {
		t.Fatalf("expected min supported deployment version to be 0.0.0, got %q", got)
	}
}

func TestDeploymentCompatibilityRules_DefaultRequirementAllowsOlderCompatibleDeployment(
	t *testing.T,
) {
	t.Parallel()

	// Given: a default-compatible command running in a newer launcher minor.
	cmd := &cobra.Command{Use: "destroy"}
	requireDefaultDeploymentCompatibility(cmd)
	req := deploymentcompatibility.Requirement{
		CommandName:                   cmd.Name(),
		MinSupportedDeploymentVersion: minSupportedDeploymentVersionFromAnnotations(cmd),
	}

	// When: a 2.1 launcher checks a deployment created by a 1.4 launcher.
	res := deploymentcompatibility.Check("1.4.1", "2.1.0", req)
	// Then: the deployment is allowed because the command has no higher minimum.
	if !res.Allowed {
		t.Fatalf("expected older compatible deployment to be allowed, got: %v", res.Err)
	}

	// When: the deployment was created by a newer launcher than the current one.
	res = deploymentcompatibility.Check("2.2.0", "2.1.0", req)
	// Then: the forward-compatibility guard still rejects it.
	if res.Allowed {
		t.Fatal("expected newer deployment to be rejected")
	}
	var inc *deploymentcompatibility.IncompatibleError
	if !errors.As(res.Err, &inc) {
		t.Fatalf("expected IncompatibleError, got %T: %v", res.Err, res.Err)
	}
	if inc.Reason != deploymentcompatibility.ReasonDeploymentNewerThanLauncher {
		t.Fatalf(
			"expected reason %q, got %q",
			deploymentcompatibility.ReasonDeploymentNewerThanLauncher,
			inc.Reason,
		)
	}
}

func TestDeploymentCompatibilityRules_MinorBaselineAndNeverNewerThanLauncher(t *testing.T) {
	t.Parallel()

	// Given: a command declares a minor-level minimum (patch is ignored) and we build
	// the compatibility requirement from the command annotations.
	cmd := &cobra.Command{Use: "deploy"}
	requireMinorBaselineDeploymentCompatibility(cmd, "1.2.5")

	req := deploymentcompatibility.Requirement{
		CommandName:                   cmd.Name(),
		MinSupportedDeploymentVersion: minSupportedDeploymentVersionFromAnnotations(cmd),
	}
	// Given: the declared minimum is normalized to the minor baseline.
	if req.MinSupportedDeploymentVersion != "1.2.0" {
		t.Fatalf("expected normalized minimum 1.2.0, got %q", req.MinSupportedDeploymentVersion)
	}

	// Given: test cases spanning the contract space: allowed versions, too-new deployments,
	// and deployments that are too old for the declared minimum.
	testCases := []struct {
		name              string
		deploymentVersion string
		launcherVersion   string
		allowed           bool
		reason            deploymentcompatibility.Reason
	}{
		{
			name:              "allows same minor at patch 0",
			deploymentVersion: "1.2.0",
			launcherVersion:   "1.2.5",
			allowed:           true,
		},
		{
			name:              "rejects deployment newer patch than launcher",
			deploymentVersion: "1.2.7",
			launcherVersion:   "1.2.5",
			allowed:           false,
			reason:            deploymentcompatibility.ReasonDeploymentNewerThanLauncher,
		},
		{
			name:              "rejects deployment newer minor than launcher",
			deploymentVersion: "1.3.0",
			launcherVersion:   "1.2.5",
			allowed:           false,
			reason:            deploymentcompatibility.ReasonDeploymentNewerThanLauncher,
		},
		{
			name:              "rejects deployment older than minimum minor",
			deploymentVersion: "1.1.9",
			launcherVersion:   "1.2.5",
			allowed:           false,
			reason:            deploymentcompatibility.ReasonDeploymentTooOld,
		},
	}

	for _, testcase := range testCases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			// When: the compatibility check compares deployment, launcher and requirement.
			res := deploymentcompatibility.Check(
				testcase.deploymentVersion,
				testcase.launcherVersion,
				req,
			)

			// Then: the result matches the rule expectations.
			if res.Allowed != testcase.allowed {
				t.Fatalf(
					"expected allowed=%v, got allowed=%v (err=%v)",
					testcase.allowed,
					res.Allowed,
					res.Err,
				)
			}
			if testcase.allowed {
				if res.Err != nil {
					t.Fatalf("expected nil error when allowed, got: %v", res.Err)
				}

				return
			}

			var inc *deploymentcompatibility.IncompatibleError
			if !errors.As(res.Err, &inc) {
				t.Fatalf("expected IncompatibleError, got %T: %v", res.Err, res.Err)
			}
			if inc.Reason != testcase.reason {
				t.Fatalf("expected reason %q, got %q", testcase.reason, inc.Reason)
			}
		})
	}
}

func TestDeploymentCompatibilityRules_StrictRevisionRequirement(t *testing.T) {
	t.Parallel()

	// Given: a command declares a strict (patch-accurate) minimum.
	cmd := &cobra.Command{Use: "deploy"}
	requireDeploymentCompatibility(cmd, "1.2.5")

	req := deploymentcompatibility.Requirement{
		CommandName:                   cmd.Name(),
		MinSupportedDeploymentVersion: minSupportedDeploymentVersionFromAnnotations(cmd),
	}

	// When: the deployment is below the strict patch minimum.
	res := deploymentcompatibility.Check("1.2.4", "1.2.5", req)
	// Then: it is rejected as too old.
	if res.Allowed {
		t.Fatal("expected disallowed")
	}
	var inc *deploymentcompatibility.IncompatibleError
	if !errors.As(res.Err, &inc) {
		t.Fatalf("expected IncompatibleError, got %T: %v", res.Err, res.Err)
	}
	if inc.Reason != deploymentcompatibility.ReasonDeploymentTooOld {
		t.Fatalf(
			"expected reason %q, got %q",
			deploymentcompatibility.ReasonDeploymentTooOld,
			inc.Reason,
		)
	}

	// When: the deployment exactly meets the strict patch requirement.
	res = deploymentcompatibility.Check("1.2.5", "1.2.5", req)
	// Then: it is allowed.
	if !res.Allowed {
		t.Fatalf("expected allowed, got: %v", res.Err)
	}
}

func TestDeploymentCompatibilityRules_OlderMinorBaselineAllowsNewerDeploymentsUpToLauncher(
	t *testing.T,
) {
	t.Parallel()

	// Given: a command declares that it still supports deployments as old as 1.1.*,
	// while the launcher is already in 1.3.*.
	cmd := &cobra.Command{Use: "status"}
	requireMinorBaselineDeploymentCompatibility(cmd, "1.1.9")

	req := deploymentcompatibility.Requirement{
		CommandName:                   cmd.Name(),
		MinSupportedDeploymentVersion: minSupportedDeploymentVersionFromAnnotations(cmd),
	}
	// Given: the declared minimum is normalized to the minor baseline.
	if req.MinSupportedDeploymentVersion != "1.1.0" {
		t.Fatalf("expected normalized minimum 1.1.0, got %q", req.MinSupportedDeploymentVersion)
	}

	// When: the deployment is from a newer minor but still not newer than the launcher.
	res := deploymentcompatibility.Check("1.2.7", "1.3.1", req)
	// Then: it is allowed.
	if !res.Allowed {
		t.Fatalf("expected allowed, got: %v", res.Err)
	}

	// When: the deployment is older than the declared minimum.
	res = deploymentcompatibility.Check("1.0.9", "1.3.1", req)
	// Then: it is rejected as too old.
	if res.Allowed {
		t.Fatal("expected disallowed")
	}
	var inc *deploymentcompatibility.IncompatibleError
	if !errors.As(res.Err, &inc) {
		t.Fatalf("expected IncompatibleError, got %T: %v", res.Err, res.Err)
	}
	if inc.Reason != deploymentcompatibility.ReasonDeploymentTooOld {
		t.Fatalf(
			"expected reason %q, got %q",
			deploymentcompatibility.ReasonDeploymentTooOld,
			inc.Reason,
		)
	}
}

func TestRequireMinorBaselineDeploymentCompatibility_DoesNotPanicOnInvalidVersion(t *testing.T) {
	t.Parallel()

	// Given: an invalid version string is used while defining command compatibility.
	cmd := &cobra.Command{Use: "deploy"}
	requireMinorBaselineDeploymentCompatibility(cmd, "not-a-version")

	// When: the runtime compatibility checker runs using the requirement stored in the command.
	req := deploymentcompatibility.Requirement{
		CommandName:                   cmd.Name(),
		MinSupportedDeploymentVersion: minSupportedDeploymentVersionFromAnnotations(cmd),
	}
	res := deploymentcompatibility.Check("1.0.0", "1.0.0", req)

	// Then: we do not panic; the invalid version is reported as a structured error.
	if res.Allowed {
		t.Fatal("expected disallowed")
	}
	var inv *deploymentcompatibility.InvalidVersionError
	if !errors.As(res.Err, &inv) {
		t.Fatalf("expected InvalidVersionError, got %T: %v", res.Err, res.Err)
	}
}
