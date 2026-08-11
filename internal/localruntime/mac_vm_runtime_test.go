// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

const (
	localTestDeploymentID     = "exasol-local-test"
	localTestClusterIdentity  = "exasol-personal;exasol-local-test;local;local"
	localTestDatabasePort     = 28563
	localTestSSHForwardedPort = 20022
)

func TestLocalRunnerVersionCheckArgs_PassesLauncherVersionCheckSettings(t *testing.T) {
	// Given
	const expectedURL = "https://example.test/v1/version-check"
	versionCheck := localinstall.VersionCheckConfig{
		Enabled:  true,
		URL:      expectedURL,
		Identity: localTestClusterIdentity,
	}

	// When
	args, err := localRunnerVersionCheckArgs(versionCheck)
	// Then
	if err != nil {
		t.Fatalf("expected version-check args, got %v", err)
	}
	expected := []string{
		"--version-check-enabled=true",
		"--version-check-url", expectedURL,
		"--version-check-identity", localTestClusterIdentity,
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected args %#v, got %#v", expected, args)
	}
}

func TestLocalRunnerVersionCheckArgs_DisablesRunnerWhenLauncherVersionCheckDisabled(
	t *testing.T,
) {
	t.Parallel()

	// Given
	// When
	args, err := localRunnerVersionCheckArgs(localinstall.VersionCheckConfig{})
	// Then
	if err != nil {
		t.Fatalf("expected disabled version-check args, got %v", err)
	}
	expected := []string{"--version-check-enabled=false"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("expected args %#v, got %#v", expected, args)
	}
}

func newTestDeploymentWithVersionCheckState(
	t *testing.T,
	versionCheckEnabled bool,
	clusterIdentity string,
) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{
		DeploymentId:        localTestDeploymentID,
		ClusterIdentity:     clusterIdentity,
		VersionCheckEnabled: versionCheckEnabled,
		DeploymentVersion:   "0.0.0",
	}
	workflowState := &config.WorkflowStateInitialized{}
	if err := state.SetWorkflowStateAndWrite(workflowState, deployment); err != nil {
		t.Fatalf("failed to write launcher state: %v", err)
	}

	return deployment
}

func TestLocalRunnerSlcArgsFromState(t *testing.T) {
	t.Parallel()

	slcs := []localinstall.SLCConfig{
		{Image: "docker.io/x:pytag", Target: "/exa/slc/python-3.12"},
		{Image: "docker.io/x:javatag", Target: "/exa/slc/java-17"},
	}

	args := localRunnerSlcArgs(slcs)

	want := []string{
		"--slc", "docker.io/x:pytag=/exa/slc/python-3.12",
		"--slc", "docker.io/x:javatag=/exa/slc/java-17",
	}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestLocalRunnerSlcArgsIncludesCustomMountAndPackage(t *testing.T) {
	t.Parallel()

	// Given
	slcs := []localinstall.SLCConfig{
		{Image: "img:py", Target: "/exa/slc/py"},
		{
			Image:   "custom:mypy3-abc",
			Target:  "/exa/slc/custom-mypy3",
			Package: "custom-mypy3-abc.tar.gz",
		},
	}

	// When
	args := localRunnerSlcArgs(slcs)
	// Then

	want := []string{
		"--slc", "img:py=/exa/slc/py",
		"--slc", "custom:mypy3-abc=/exa/slc/custom-mypy3",
		"--slc-package", "custom:mypy3-abc=custom-mypy3-abc.tar.gz",
	}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("expected official and custom args, got %v", args)
	}
}

func TestLocalRunnerSlcArgsOmitsPackageForOfficialSLCs(t *testing.T) {
	t.Parallel()

	// Given
	slcs := []localinstall.SLCConfig{
		{Image: "img:py", Target: "/exa/slc/py"},
	}

	// When
	args := localRunnerSlcArgs(slcs)
	// Then
	for _, arg := range args {
		if arg == "--slc-package" {
			t.Fatalf("official SLCs must not be import-delivered: %v", args)
		}
	}
}

//nolint:paralleltest // serial package; see note at top of file
func TestStart_InvokesResolvedRunnerWithArgs(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := newTestDeploymentWithVersionCheckState(t, false, "")
	localRuntime := NewMacVMRuntime(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}
	runnerScript := "#!/bin/sh\nprintf '%s\\n' \"$*\"\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime = NewMacVMRuntime(deployment, manager)
	var out bytes.Buffer

	// When
	err := localRuntime.Start(context.Background(), &out, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "auto"},
		CPUCount:      2,
		MemoryMB:      4096,
		DataSizeGB:    100,
	})
	// Then
	if err != nil {
		t.Fatalf("expected Start to succeed, got %v", err)
	}
	const expected = "start --version-check-enabled=false --ports auto 2 4096 100"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected args %q, got %q", expected, out.String())
	}
}
