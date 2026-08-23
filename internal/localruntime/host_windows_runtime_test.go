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

func TestWindowsHostRuntime_Prepare_PodmanAlreadyPresentSkipsWinget(t *testing.T) {
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "podman", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	rt := NewHostWindowsRuntime(deployment, nil)

	var out, outErr bytes.Buffer
	if err := rt.Prepare(context.Background(), nil, &out, &outErr); err != nil {
		t.Fatalf("Prepare() unexpected error: %v", err)
	}
	if strings.Contains(out.String(), "install") {
		t.Errorf("did not expect install messaging when podman is present, got %q", out.String())
	}
	if _, err := os.Stat(rt.paths.WorkDir); err != nil {
		t.Errorf("expected work dir to be created, got %v", err)
	}
}

func TestWindowsHostRuntime_Prepare_RegistryRefreshRecoversStalePath(t *testing.T) {
	// podman is not on the initial PATH, but the "registry" override
	// points at a directory that contains podman. Prepare must recover
	// via the pre-install refresh and never invoke winget.
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `echo "should not run" >&2; exit 1`)

	podmanDir := t.TempDir()
	dropShim(t, podmanDir, "podman", `exit 0`)
	t.Setenv("EXASOL_WINGET_TEST_PODMAN_PATH_ADDITIONS", podmanDir)

	deployment := config.NewDeploymentDir(t.TempDir())
	rt := NewHostWindowsRuntime(deployment, nil)

	var out, outErr bytes.Buffer
	if err := rt.Prepare(context.Background(), nil, &out, &outErr); err != nil {
		t.Fatalf("Prepare() unexpected error: %v", err)
	}
	if _, err := os.Stat(wingetLog); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("winget must not have been invoked; log exists (err=%v)", err)
	}
	if strings.Contains(out.String(), "will install") {
		t.Errorf("did not expect install messaging on recovery, got %q", out.String())
	}
}

func TestWindowsHostRuntime_Prepare_InvokesWingetWhenPodmanMissing(t *testing.T) {
	// No podman anywhere. Fake winget exits 0 without producing podman
	// (we cannot fake Windows registry write from a shell shim). Prepare
	// must invoke winget with the exact argv AND return the specific
	// "installed but not on PATH" error so users get a clear message
	// when a real install silently fails to register a PATH entry.
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	rt := NewHostWindowsRuntime(deployment, nil)

	var out, outErr bytes.Buffer
	err := rt.Prepare(context.Background(), nil, &out, &outErr)
	if err == nil {
		t.Fatal("expected error when winget succeeds but podman remains missing")
	}
	if !strings.Contains(err.Error(), "podman is still not on PATH") {
		t.Errorf("expected 'still not on PATH' error, got %v", err)
	}
	if !strings.Contains(out.String(), winget.PodmanInstallCommand()) {
		t.Errorf("expected exact install command shown to user, got %q", out.String())
	}
	logged, readErr := os.ReadFile(wingetLog)
	if readErr != nil {
		t.Fatalf("read winget log: %v", readErr)
	}
	if !strings.Contains(string(logged), "install --exact --id RedHat.Podman") {
		t.Errorf("expected winget install argv logged, got %q", string(logged))
	}
}

func TestWindowsHostRuntime_Prepare_WingetMissingProducesHelpfulError(t *testing.T) {
	clearPodmanPathOverride(t)
	newIsolatedShimDir(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	rt := NewHostWindowsRuntime(deployment, nil)

	var out, outErr bytes.Buffer
	err := rt.Prepare(context.Background(), nil, &out, &outErr)
	if err == nil {
		t.Fatal("expected error when both podman and winget are missing")
	}
	if !errors.Is(err, winget.ErrWingetNotFound) {
		t.Errorf("expected ErrWingetNotFound, got %v", err)
	}
	if !strings.Contains(outErr.String(), "App Installer") {
		t.Errorf("expected user-facing App Installer guidance, got %q", outErr.String())
	}
}

func TestWindowsHostRuntime_Prepare_WingetFailurePrintsManualFallback(t *testing.T) {
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "winget", `echo "download failed" >&2; exit 1`)

	deployment := config.NewDeploymentDir(t.TempDir())
	rt := NewHostWindowsRuntime(deployment, nil)

	var out, outErr bytes.Buffer
	err := rt.Prepare(context.Background(), nil, &out, &outErr)
	if err == nil {
		t.Fatal("expected error when winget install fails")
	}
	if !strings.Contains(outErr.String(), "manually") {
		t.Errorf("expected manual install fallback message, got %q", outErr.String())
	}
	if !strings.Contains(outErr.String(), "https://podman.io/") {
		t.Errorf("expected podman.io link in fallback message, got %q", outErr.String())
	}
}

func TestWindowsHostRuntime_Prepare_NonTerminalStdinAutoInstalls(t *testing.T) {
	// A non-terminal io.Reader (bytes.Buffer here) must be treated as
	// scripted / non-interactive: the launcher must not prompt, and the
	// install must proceed with the default consent (yes).
	clearPodmanPathOverride(t)
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `exit 0`)

	deployment := config.NewDeploymentDir(t.TempDir())
	rt := NewHostWindowsRuntime(deployment, nil)

	var out, outErr bytes.Buffer
	// A bytes.Buffer is not *os.File so prompt.IsTerminal reports false.
	err := rt.Prepare(context.Background(), &bytes.Buffer{}, &out, &outErr)
	if err == nil {
		t.Fatal("expected 'still not on PATH' error, got nil")
	}
	logged, readErr := os.ReadFile(wingetLog)
	if readErr != nil {
		t.Fatalf("read winget log: %v", readErr)
	}
	if !strings.Contains(string(logged), "install --exact --id RedHat.Podman") {
		t.Errorf("winget must have been invoked despite non-terminal stdin, got %q",
			string(logged))
	}
	if strings.Contains(out.String(), "[Y/n]") {
		t.Errorf("no interactive [Y/n] prompt allowed on non-terminal stdin, got %q",
			out.String())
	}
}
