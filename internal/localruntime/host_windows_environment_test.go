// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/winget"
)

// newIsolatedShimDir creates a fresh temp directory and sets PATH to it
// exclusively. Callers seed shims into the returned directory via
// dropShim. Isolating PATH is essential: the developer's real podman or
// winget must not leak into these tests.
func newIsolatedShimDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake binary shims are POSIX shell scripts; skipping on windows")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	return dir
}

// dropShim writes a shell shim named `name` into `dir`, prefixed with an
// argv-log line. Returns the log path so tests can assert argv.
// The shim body must use only shell built-ins (printf, echo, redirects)
// because the isolated PATH does not include /bin or /usr/bin.
func dropShim(t *testing.T, dir, name, body string) string {
	t.Helper()
	logPath := filepath.Join(dir, name+".argv.log")
	script := "#!/bin/sh\n" +
		`printf '%s' "` + name + `" >> "` + logPath + `"` + "\n" +
		`for arg in "$@"; do printf ' %s' "$arg" >> "` + logPath + `"; done` + "\n" +
		`printf '\n' >> "` + logPath + `"` + "\n" +
		body + "\n"
	//nolint:gosec // Test fixture must be executable.
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s shim: %v", name, err)
	}

	return logPath
}

// clearPodmanPathOverride pins the winget package's PATH-additions test
// hook to an empty value so EnsurePodmanOnPath treats it as a present
// but empty override and no-ops. Leaving it truly unset would make the
// non-windows guard in winget.podmanPathAdditions fire and surface a
// test-only error into Prepare.
func clearPodmanPathOverride(t *testing.T) {
	t.Helper()
	t.Setenv("EXASOL_WINGET_TEST_PODMAN_PATH_ADDITIONS", "")
}

// recordingApprover captures the requests it is asked to approve, so tests
// can assert both the decision and the exact commands shown to the user.
type recordingApprover struct {
	approve  bool
	err      error
	requests []HostChangeRequest
	progress bytes.Buffer
}

func (approver *recordingApprover) options() PrepareOptions {
	return PrepareOptions{
		ApproveHostChange: func(
			_ context.Context, request HostChangeRequest,
		) (bool, error) {
			approver.requests = append(approver.requests, request)

			return approver.approve, approver.err
		},
		Progress: &approver.progress,
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_PodmanAlreadyPresentSkipsWinget(t *testing.T) {
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "podman", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	if err := hostRuntime.Prepare(
		context.Background(), nil, nil, approver.options(),
	); err != nil {
		t.Fatalf("Prepare() unexpected error: %v", err)
	}
	if len(approver.requests) != 0 {
		t.Errorf("no approval should be requested when podman is present, got %+v",
			approver.requests)
	}
	if _, err := os.Stat(hostRuntime.paths.WorkDir); err != nil {
		t.Errorf("expected work dir to be created, got %v", err)
	}
}

func TestWindowsPrepare_RegistryRefreshRecoversStalePath(t *testing.T) {
	// podman is not on the initial PATH, but the "registry" override
	// points at a directory that contains podman. Prepare must recover
	// via the pre-install refresh, never invoking winget or asking for
	// approval to reinstall something already present.
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `echo "should not run" >&2; exit 1`)

	podmanDir := t.TempDir()
	dropShim(t, podmanDir, "podman", `exit 0`)
	t.Setenv("EXASOL_WINGET_TEST_PODMAN_PATH_ADDITIONS", podmanDir)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	if err := hostRuntime.Prepare(
		context.Background(), nil, nil, approver.options(),
	); err != nil {
		t.Fatalf("Prepare() unexpected error: %v", err)
	}
	if _, err := os.Stat(wingetLog); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("winget must not have been invoked; log exists (err=%v)", err)
	}
	if len(approver.requests) != 0 {
		t.Errorf("no approval should be requested on PATH recovery, got %+v",
			approver.requests)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_ApprovedInstallInvokesWinget(t *testing.T) {
	// No podman anywhere. Fake winget exits 0 without producing podman
	// (we cannot fake a Windows registry write from a shell shim), so
	// Prepare must surface the specific "still not on PATH" error after
	// running the approved command.
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when winget succeeds but podman remains missing")
	}
	if !strings.Contains(err.Error(), "still not on PATH") {
		t.Errorf("expected 'still not on PATH' error, got %v", err)
	}
	if len(approver.requests) != 1 {
		t.Fatalf("expected exactly one approval request, got %+v", approver.requests)
	}
	request := approver.requests[0]
	if request.Kind != HostChangeInstallContainerRuntime {
		t.Errorf("unexpected change kind %q", request.Kind)
	}
	// The user must be shown the same command that actually runs.
	if len(request.Commands) != 1 ||
		!strings.Contains(request.Commands[0].String(), "install --exact --id RedHat.Podman") {
		t.Errorf("expected the winget install command in the request, got %+v", request.Commands)
	}
	logged, readErr := os.ReadFile(wingetLog)
	if readErr != nil {
		t.Fatalf("read winget log: %v", readErr)
	}
	if !strings.Contains(string(logged), "install --exact --id RedHat.Podman") {
		t.Errorf("expected winget install argv logged, got %q", string(logged))
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_DeclinedInstallDoesNotRunWinget(t *testing.T) {
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)
	approver := &recordingApprover{approve: false}

	err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when the host change is declined")
	}
	if !strings.Contains(err.Error(), "was not approved") {
		t.Errorf("expected a 'not approved' error, got %v", err)
	}
	if _, statErr := os.Stat(wingetLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("declining must not run winget; log exists (err=%v)", statErr)
	}
}

// Regression guard: preparation used to prompt from inside the runtime and
// silently default to "yes" whenever stdin was not a terminal, which meant a
// scripted run would install Podman with no approval at all. A missing
// approver must now deny the change instead.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_MissingApproverDeniesInstall(t *testing.T) {
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)

	var progress bytes.Buffer
	err := hostRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{
		Progress: &progress,
	})
	if err == nil {
		t.Fatal("expected error when no approver is supplied")
	}
	if !strings.Contains(err.Error(), "requires explicit approval") {
		t.Errorf("expected an 'explicit approval' error, got %v", err)
	}
	if _, statErr := os.Stat(wingetLog); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("a missing approver must not run winget; log exists (err=%v)", statErr)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_WingetMissingProducesHelpfulError(t *testing.T) {
	clearPodmanPathOverride(t)
	newIsolatedShimDir(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when both podman and winget are missing")
	}
	if !errors.Is(err, winget.ErrWingetNotFound) {
		t.Errorf("expected ErrWingetNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "App Installer") {
		t.Errorf("expected App Installer guidance in the error, got %v", err)
	}
	// Nothing can be installed, so there is nothing to approve.
	if len(approver.requests) != 0 {
		t.Errorf("no approval should be requested without winget, got %+v", approver.requests)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_WingetFailureExplainsManualFallback(t *testing.T) {
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "winget", `echo "download failed" >&2; exit 1`)

	deployment := config.NewDeploymentDir(t.TempDir())
	hostRuntime := NewHostWindowsRuntime(deployment, nil)
	approver := &recordingApprover{approve: true}

	err := hostRuntime.Prepare(context.Background(), nil, nil, approver.options())
	if err == nil {
		t.Fatal("expected error when winget install fails")
	}
	if !strings.Contains(err.Error(), "manually") ||
		!strings.Contains(err.Error(), "https://podman.io/") {
		t.Errorf("expected a manual-install fallback with the podman.io link, got %v", err)
	}
}

// The reconciliation path depends on this: with the machine stopped, Status
// cannot tell a stopped container from a machine it cannot reach, so the
// runtime has to be made observable first.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsEnsureQueryable_StartsStoppedMachine(t *testing.T) {
	dir := newIsolatedShimDir(t)
	shim := `if [ "$1 $2" = "machine list" ]; then ` +
		`printf 'podman-machine-default\n'; exit 0; fi
if [ "$1 $2" = "machine inspect" ]; then printf 'stopped\n'; exit 0; fi
if [ "$1 $2" = "machine start" ]; then exit 0; fi
exit 1`
	logPath := dropShim(t, dir, "podman", shim)

	hostRuntime := NewHostWindowsRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	var out, outErr bytes.Buffer
	if err := hostRuntime.EnsureQueryable(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("EnsureQueryable() unexpected error: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read podman log: %v", err)
	}
	if !strings.Contains(string(logged), "machine start") {
		t.Errorf("expected the stopped machine to be started, got %q", string(logged))
	}
	if !strings.Contains(out.String(), "starting it") {
		t.Errorf("expected progress explaining the pause, got %q", out.String())
	}
}

// Regression: PATH was only refreshed inside Prepare, which stop, destroy,
// status and diagnostics never call. After the launcher installed Podman, every
// later command in the same terminal failed with "executable file not found in
// %PATH%", because Windows fixes a process's PATH at start-up.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsEnsureQueryable_RecoversPodmanMissingFromProcessPath(t *testing.T) {
	// Given podman absent from this process's PATH but present where the
	// registry says it is — the state of a terminal opened before the install
	newIsolatedShimDir(t)
	podmanDir := t.TempDir()
	shim := `if [ "$1 $2" = "machine list" ]; then ` +
		`printf 'podman-machine-default\n'; exit 0; fi
if [ "$1 $2" = "machine inspect" ]; then printf 'running\n'; exit 0; fi
exit 1`
	logPath := dropShim(t, podmanDir, "podman", shim)
	t.Setenv("EXASOL_WINGET_TEST_PODMAN_PATH_ADDITIONS", podmanDir)

	hostRuntime := NewHostWindowsRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	// When the runtime is asked to become observable
	if err := hostRuntime.EnsureQueryable(context.Background(), nil, nil); err != nil {
		t.Fatalf("EnsureQueryable() unexpected error: %v", err)
	}

	// Then podman was found and used rather than reported missing
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("expected podman to be resolved and invoked, got %v", err)
	}
}

// The same recovery has to apply to container commands, which reach podman
// through the execution environment rather than the machine helpers.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsExecutionEnvironment_ResolvesPodmanMissingFromProcessPath(t *testing.T) {
	newIsolatedShimDir(t)
	podmanDir := t.TempDir()
	logPath := dropShim(t, podmanDir, "podman", `exit 0`)
	t.Setenv("EXASOL_WINGET_TEST_PODMAN_PATH_ADDITIONS", podmanDir)

	environment := windowsHostEnvironmentPreparer{}.NewExecutionEnvironment(nil)

	if err := environment.Run(
		context.Background(), nil, nil, nil, "podman", "rm", "--force", "some-container",
	); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected podman to be resolved and invoked: %v", err)
	}
	if !strings.Contains(string(logged), "rm --force some-container") {
		t.Errorf("expected the container command to reach podman, got %q", string(logged))
	}
}

func TestWindowsRuntime_ReportsWindowsPlatform(t *testing.T) {
	t.Parallel()

	hostRuntime := NewHostWindowsRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	if hostRuntime.Platform() != HostPlatformWindows {
		t.Errorf("expected the Windows platform, got %q", hostRuntime.Platform())
	}
}
