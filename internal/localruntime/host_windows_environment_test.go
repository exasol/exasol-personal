// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// newIsolatedShimDir creates a fresh temp directory and sets PATH to it
// exclusively. Callers seed shims into the returned directory via
// dropShim. Isolating PATH is essential: the developer's real container
// engine must not leak into these tests.
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

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsPrepare_FailsWithoutDockerOnPath(t *testing.T) {
	newIsolatedShimDir(t)
	localRuntime := NewHostWindowsRuntime(newLinuxRuntimeStatusDeployment(t), nil)

	err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{})
	if err == nil {
		t.Fatal("expected Prepare to fail without docker on PATH")
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsContainerHostRunning_ReflectsDockerInfo(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		running bool
	}{
		{name: "daemon answers", body: "exit 0", running: true},
		{name: "daemon stopped", body: "exit 1", running: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := newIsolatedShimDir(t)
			dropShim(t, dir, "docker", test.body)

			running, err := windowsHostEnvironmentPreparer{}.ContainerHostRunning(
				context.Background(),
			)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if running != test.running {
				t.Fatalf("expected running=%t, got %t", test.running, running)
			}
		})
	}
}

func TestWindowsExecutionEnvironment_SyncRunsNothing(t *testing.T) {
	t.Parallel()
	environment := windowsHostEnvironmentPreparer{}.NewExecutionEnvironment(
		[]string{"/nonexistent/engine"},
	)
	var stdout, stderr bytes.Buffer

	if err := environment.Sync(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("expected Sync to succeed without running a command, got %v", err)
	}
}
