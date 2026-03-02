// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploymentcompatibility

import (
	"errors"
	"strings"
	"testing"
)

func TestCheck_AllowsSupportedVersions(t *testing.T) {
	t.Parallel()
	res := Check(
		"1.2.0",
		"2.0.0",
		Requirement{CommandName: "status", MinSupportedDeploymentVersion: "1.0.0"},
	)
	if !res.Allowed {
		t.Fatalf("expected allowed, got disallowed error: %v", res.Err)
	}
	if res.Err != nil {
		t.Fatalf("expected nil error when allowed, got: %v", res.Err)
	}
}

func TestCheck_RejectsDeploymentNewerThanLauncher_OnPrinciple(t *testing.T) {
	t.Parallel()
	res := Check(
		"2.0.0",
		"1.9.0",
		Requirement{CommandName: "deploy", MinSupportedDeploymentVersion: "1.0.0"},
	)
	if res.Allowed {
		t.Fatal("expected disallowed")
	}

	var inc *IncompatibleError
	if !errors.As(res.Err, &inc) {
		t.Fatalf("expected IncompatibleError, got %T: %v", res.Err, res.Err)
	}
	if inc.Reason != ReasonDeploymentNewerThanLauncher {
		t.Fatalf("expected reason %q, got %q", ReasonDeploymentNewerThanLauncher, inc.Reason)
	}
	if inc.RequiredAction != ActionUpgradeLauncher {
		t.Fatalf("expected action %q, got %q", ActionUpgradeLauncher, inc.RequiredAction)
	}
	if !strings.Contains(inc.Error(), ">= 2.0.0") {
		t.Fatalf(
			"expected error to mention minimum upgrade version (>= deployment version), got: %q",
			inc.Error(),
		)
	}
}

func TestCheck_IgnoresPrereleaseSuffixes(t *testing.T) {
	t.Parallel()

	req := Requirement{CommandName: "deploy", MinSupportedDeploymentVersion: "1.0.0"}

	// Deployment created by release, launcher is an RC of the same version.
	res := Check("1.2.0", "1.2.0-rc1", req)
	if !res.Allowed {
		t.Fatalf("expected allowed, got error: %v", res.Err)
	}

	// Deployment created by an RC, launcher is the final release.
	res = Check("1.2.0-rc2", "1.2.0", req)
	if !res.Allowed {
		t.Fatalf("expected allowed, got error: %v", res.Err)
	}
}

func TestCheck_RejectsDeploymentOlderThanCommandMinimum(t *testing.T) {
	t.Parallel()
	res := Check(
		"1.0.0",
		"2.0.0",
		Requirement{CommandName: "deploy", MinSupportedDeploymentVersion: "1.5.0"},
	)
	if res.Allowed {
		t.Fatal("expected disallowed")
	}

	var inc *IncompatibleError
	if !errors.As(res.Err, &inc) {
		t.Fatalf("expected IncompatibleError, got %T: %v", res.Err, res.Err)
	}
	if inc.Reason != ReasonDeploymentTooOld {
		t.Fatalf("expected reason %q, got %q", ReasonDeploymentTooOld, inc.Reason)
	}
	if inc.RequiredAction != ActionUseCompatibleLauncher {
		t.Fatalf("expected action %q, got %q", ActionUseCompatibleLauncher, inc.RequiredAction)
	}
}

func TestCheck_RejectsInvalidVersions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		deployment   string
		launcher     string
		minSupported string
	}{
		{name: "missing deployment", deployment: "", launcher: "1.0.0", minSupported: "1.0.0"},
		{
			name:         "invalid deployment",
			deployment:   "not-a-version",
			launcher:     "1.0.0",
			minSupported: "1.0.0",
		},
		{name: "invalid launcher", deployment: "1.0.0", launcher: "nope", minSupported: "1.0.0"},
		{name: "invalid minimum", deployment: "1.0.0", launcher: "1.0.0", minSupported: "bad"},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()
			res := Check(
				testcase.deployment,
				testcase.launcher,
				Requirement{CommandName: "x", MinSupportedDeploymentVersion: testcase.minSupported},
			)
			if res.Allowed {
				t.Fatal("expected disallowed")
			}
			var inv *InvalidVersionError
			if !errors.As(res.Err, &inv) {
				t.Fatalf("expected InvalidVersionError, got %T: %v", res.Err, res.Err)
			}
		})
	}
}
