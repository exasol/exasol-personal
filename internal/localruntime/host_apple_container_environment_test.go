// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/applecontainer"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts/runtimeartifactstest"
)

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestAppleContainerPrepare_CLIAlreadyPresentSkipsInstall(t *testing.T) {
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, applecontainer.CLIBinary, `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostAppleContainerRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	if err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options()); err != nil {
		t.Fatalf("expected Prepare to succeed, got %v", err)
	}
	if len(approver.requests) != 0 {
		t.Fatalf("expected no approval requests, got %+v", approver.requests)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestAppleContainerPrepare_InstallsViaBrewWhenApproved(t *testing.T) {
	dir := newIsolatedShimDir(t)
	brewLog := dropShim(t, dir, "brew", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostAppleContainerRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	// The fake brew cannot actually place a `container` binary on PATH, so
	// Prepare must surface the specific "still not on PATH" error after
	// running the approved install command, the same shape the Windows
	// winget test uses for the equivalent limitation.
	err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when brew succeeds but `container` remains missing")
	}
	if !strings.Contains(err.Error(), "still not on PATH") {
		t.Errorf("expected 'still not on PATH' error, got %v", err)
	}
	if len(approver.requests) != 1 {
		t.Fatalf("expected exactly one approval request, got %+v", approver.requests)
	}
	request := approver.requests[0]
	if request.Kind != HostChangeInstallAppleContainer {
		t.Errorf("unexpected change kind %q", request.Kind)
	}
	if len(request.Commands) != 1 || !strings.Contains(request.Commands[0].String(), "brew install container") {
		t.Errorf("expected the brew install command in the request, got %+v", request.Commands)
	}
	logged, readErr := os.ReadFile(brewLog)
	if readErr != nil {
		t.Fatalf("read brew log: %v", readErr)
	}
	if !strings.Contains(string(logged), "install container") {
		t.Errorf("expected brew install argv logged, got %q", string(logged))
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestAppleContainerPrepare_DeclinedInstallDoesNotRunBrew(t *testing.T) {
	dir := newIsolatedShimDir(t)
	brewLog := dropShim(t, dir, "brew", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostAppleContainerRuntime(deployment, nil)
	approver := &recordingApprover{approve: false}

	err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when the host change is declined")
	}
	if !strings.Contains(err.Error(), "was not approved") {
		t.Errorf("expected a 'not approved' error, got %v", err)
	}
	if _, statErr := os.Stat(brewLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("declining must not run brew; log exists (err=%v)", statErr)
	}
}

// Regression guard mirroring the Windows preparer: a missing approver must
// deny the change rather than silently install.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestAppleContainerPrepare_MissingApproverDeniesInstall(t *testing.T) {
	dir := newIsolatedShimDir(t)
	brewLog := dropShim(t, dir, "brew", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostAppleContainerRuntime(deployment, nil)

	err := hostRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{})
	if err == nil {
		t.Fatal("expected error when no approver is supplied")
	}
	if !strings.Contains(err.Error(), "requires explicit approval") {
		t.Errorf("expected an explicit-approval error, got %v", err)
	}
	if _, statErr := os.Stat(brewLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("a denied change must not run brew; log exists (err=%v)", statErr)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestAppleContainerPrepare_FallsBackToSignedInstallerWhenBrewUnavailable(t *testing.T) {
	dir := newIsolatedShimDir(t)
	sudoLog := dropShim(t, dir, "sudo", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostAppleContainerRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}
	ctx := newFakeAppleContainerInstallerContext(t)

	err := hostRuntime.Prepare(ctx, nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when the installer succeeds but `container` remains missing")
	}
	if !strings.Contains(err.Error(), "still not on PATH") {
		t.Errorf("expected 'still not on PATH' error, got %v", err)
	}
	if len(approver.requests) != 1 {
		t.Fatalf("expected exactly one approval request, got %+v", approver.requests)
	}
	request := approver.requests[0]
	if request.Kind != HostChangeInstallAppleContainer {
		t.Errorf("unexpected change kind %q", request.Kind)
	}
	if len(request.Commands) != 1 || request.Commands[0].Name != "sudo" ||
		!strings.Contains(request.Commands[0].String(), "installer -pkg") {
		t.Errorf("expected the signed installer command in the request, got %+v", request.Commands)
	}
	logged, readErr := os.ReadFile(sudoLog)
	if readErr != nil {
		t.Fatalf("read sudo log: %v", readErr)
	}
	if !strings.Contains(string(logged), "installer -pkg") {
		t.Errorf("expected sudo installer argv logged, got %q", string(logged))
	}
}

// newFakeAppleContainerInstallerContext resolves the pinned installer
// resource to a local, instantly-available file instead of a real network
// download, so the test exercises ResolveInstallerPackage without network
// access. installAppleContainerViaPackage reads the manager from context,
// the same way production code does (wired once at cmd/exasol/root.go),
// rather than from the HostRuntime's own manager field.
func newFakeAppleContainerInstallerContext(t *testing.T) context.Context {
	t.Helper()
	root := t.TempDir()
	packagePath := filepath.Join(root, "installer.pkg")
	if err := os.WriteFile(packagePath, []byte("fake installer package"), 0o600); err != nil {
		t.Fatalf("failed to write fake installer package: %v", err)
	}

	return runtimeartifactstest.NewManagerContext(t, runtimeartifacts.ResourceSpec{
		applecontainer.InstallerResourceID: {
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"any": {URL: packagePath},
			},
		},
	})
}
