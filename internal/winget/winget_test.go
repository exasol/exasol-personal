// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package winget

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// windowsGOOS names the platform these guards compare against; the fake
// shims below are POSIX shell scripts and cannot run on Windows.
const windowsGOOS = "windows"

// installFakeWinget drops a POSIX shell script named "winget" into a
// fresh temp directory, prepends that directory to PATH, and returns the
// path of an argv-log file the shim writes to. Mirrors the fake-podman
// pattern used elsewhere in this repo.
func installFakeWinget(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == windowsGOOS {
		t.Skip("fake winget shim uses a POSIX shell script; skipping on windows")
	}

	dir := t.TempDir()
	argvLogPath := filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\" >> \"" + argvLogPath + "\"; done\n" +
		"printf -- '---\\n' >> \"" + argvLogPath + "\"\n" +
		body + "\n"
	binPath := filepath.Join(dir, "winget")
	//nolint:gosec // Test fixture must be executable.
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake winget shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return argvLogPath
}

func readArgvCalls(t *testing.T, logPath string) [][]string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read argv log: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	calls := make([][]string, 0, len(lines))
	current := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "---" {
			calls = append(calls, current)
			current = make([]string, 0, len(lines))

			continue
		}
		if line == "" {
			continue
		}
		current = append(current, line)
	}

	return calls
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

//nolint:paralleltest // The test replaces process-wide PATH or env.
func TestInstallPodman_Success(t *testing.T) {
	logPath := installFakeWinget(t, `printf 'Downloading...\nInstalled.\n'; exit 0`)
	var out, outErr bytes.Buffer
	if err := InstallPodman(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("InstallPodman() unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Downloading") {
		t.Errorf("expected winget stdout streamed to out, got %q", out.String())
	}
	calls := readArgvCalls(t, logPath)
	if len(calls) != 1 {
		t.Fatalf("expected 1 winget call, got %d: %v", len(calls), calls)
	}
	want := []string{
		"install",
		"--exact", "--id", "RedHat.Podman",
		"--source", "winget",
		"--accept-source-agreements",
		"--accept-package-agreements",
	}
	if !slicesEqual(calls[0], want) {
		t.Errorf("winget argv:\n  want: %v\n  got:  %v", want, calls[0])
	}
}

//nolint:paralleltest // The test replaces process-wide PATH or env.
func TestPodmanInstallCommand(t *testing.T) {
	got := PodmanInstallCommand()
	want := "winget install --exact --id RedHat.Podman --source winget " +
		"--accept-source-agreements --accept-package-agreements"
	if got != want {
		t.Errorf("PodmanInstallCommand() =\n  %q\nwant\n  %q", got, want)
	}
}

func TestInstallPodman_WingetMissingReturnsSentinel(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("cannot reliably clear winget from PATH on windows")
	}
	t.Setenv("PATH", t.TempDir())

	err := InstallPodman(context.Background(), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when winget is not on PATH")
	}
	if !errors.Is(err, ErrWingetNotFound) {
		t.Errorf("expected ErrWingetNotFound, got %v", err)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH or env.
func TestInstallPodman_WingetExitsNonZeroStreamsStderr(t *testing.T) {
	installFakeWinget(t, `echo "package not found" >&2; exit 1`)
	var out, outErr bytes.Buffer

	err := InstallPodman(context.Background(), &out, &outErr)
	if err == nil {
		t.Fatal("expected error when winget exits non-zero")
	}
	if !strings.Contains(err.Error(), "winget install RedHat.Podman failed") {
		t.Errorf("error should identify the failed install: %v", err)
	}
	if !strings.Contains(outErr.String(), "package not found") {
		t.Errorf("expected winget stderr streamed to outErr, got %q", outErr.String())
	}
}

func TestLookupWinget_MissingReturnsSentinel(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("cannot reliably clear winget from PATH on windows")
	}
	t.Setenv("PATH", t.TempDir())

	err := LookupWinget()
	if !errors.Is(err, ErrWingetNotFound) {
		t.Errorf("expected ErrWingetNotFound, got %v", err)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH or env.
func TestLookupWinget_PresentReturnsNil(t *testing.T) {
	installFakeWinget(t, `exit 0`)
	if err := LookupWinget(); err != nil {
		t.Errorf("expected nil when winget is on PATH, got %v", err)
	}
}

func TestEnsurePodmanOnPath_PrependsNewEntry(t *testing.T) {
	installDir := t.TempDir()
	t.Setenv(podmanPathAdditionsOverrideEnv, installDir)
	t.Setenv("PATH", "/some/existing/path")

	if err := EnsurePodmanOnPath(context.Background()); err != nil {
		t.Fatalf("EnsurePodmanOnPath(context.Background()) unexpected error: %v", err)
	}

	got := os.Getenv("PATH")
	wantPrefix := installDir + string(os.PathListSeparator)
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("PATH prefix: want %q, got %q", wantPrefix, got)
	}
	if !strings.Contains(got, "/some/existing/path") {
		t.Errorf("PATH did not preserve existing entries: %q", got)
	}
}

func TestEnsurePodmanOnPath_DropsDuplicates(t *testing.T) {
	sep := string(os.PathListSeparator)
	installDir := t.TempDir()
	otherDir := t.TempDir()
	t.Setenv("PATH", "/pre-existing"+sep+otherDir)
	t.Setenv(podmanPathAdditionsOverrideEnv,
		installDir+sep+otherDir+sep+"/system32")

	if err := EnsurePodmanOnPath(context.Background()); err != nil {
		t.Fatalf("EnsurePodmanOnPath(context.Background()) unexpected error: %v", err)
	}
	got := os.Getenv("PATH")
	if !strings.Contains(got, installDir) {
		t.Errorf("expected new install dir in PATH: %q", got)
	}
	if strings.Count(strings.ToLower(got), strings.ToLower(otherDir)) != 1 {
		t.Errorf("duplicate entry %q appeared more than once in PATH: %q", otherDir, got)
	}
}

func TestEnsurePodmanOnPath_EmptyAdditionsIsNoOp(t *testing.T) {
	original := "/some/pre-existing/path"
	t.Setenv("PATH", original)
	t.Setenv(podmanPathAdditionsOverrideEnv, "")

	if err := EnsurePodmanOnPath(context.Background()); err != nil {
		t.Fatalf("EnsurePodmanOnPath(context.Background()) unexpected error: %v", err)
	}
	if got := os.Getenv("PATH"); got != original {
		t.Errorf("PATH mutated on empty additions: want %q, got %q", original, got)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH or env.
func TestEnsurePodmanOnPath_NonWindowsWithoutOverrideFails(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("this guard only fires on non-windows")
	}
	if err := os.Unsetenv(podmanPathAdditionsOverrideEnv); err != nil {
		t.Fatalf("unset override env: %v", err)
	}

	err := EnsurePodmanOnPath(context.Background())
	if err == nil {
		t.Fatal("expected error on non-windows without override")
	}
	if !strings.Contains(err.Error(), podmanPathAdditionsOverrideEnv) {
		t.Errorf("error should mention the required test override env, got %v", err)
	}
}
