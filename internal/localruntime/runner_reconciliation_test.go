// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/exasol/exasol-personal/internal/config"
)

func newTestRuntimeForReconciliation(t *testing.T) *Runtime {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	testRuntime := New(deployment, nil)
	if err := os.MkdirAll(testRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}

	return testRuntime
}

func seedVersionMarker(t *testing.T, testRuntime *Runtime, version string) {
	t.Helper()

	parsed, err := semver.ParseTolerant(version)
	if err != nil {
		t.Fatalf("failed to parse test marker version %q: %v", version, err)
	}
	markerPath := testRuntime.paths.RunnerVersionMarkerPath
	if err := writeRunnerVersionMarker(markerPath, parsed); err != nil {
		t.Fatalf("failed to seed version marker: %v", err)
	}
}

func assertMarkerVersion(t *testing.T, testRuntime *Runtime, expected string) {
	t.Helper()

	actual, err := readRunnerVersionMarker(testRuntime.paths.RunnerVersionMarkerPath)
	if err != nil {
		t.Fatalf("expected a readable version marker, got %v", err)
	}
	if actual.String() != expected {
		t.Fatalf("expected marker version %q, got %q", expected, actual.String())
	}
}

func writeRunnerScript(t *testing.T, version string) string {
	t.Helper()
	requirePOSIXRunnerTest(t)

	path := filepath.Join(t.TempDir(), "launcher")
	writeExecutableTestFile(t, path, versionedRunner(version, "runner"))

	return path
}

func TestReconcileRunnerVersion_InitializesMissingMarker(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	runnerPath := writeRunnerScript(t, "1.2.3")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected marker initialization to succeed, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.3")
}

func TestReconcileRunnerVersion_ReplacesInvalidMarker(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	invalidMarker := []byte("not json")
	markerPath := testRuntime.paths.RunnerVersionMarkerPath
	if err := os.WriteFile(markerPath, invalidMarker, markerFileMode); err != nil {
		t.Fatalf("failed to seed invalid marker: %v", err)
	}
	runnerPath := writeRunnerScript(t, "1.2.3")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected invalid marker to be replaced, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.3")
}

func TestReconcileRunnerVersion_UpdatesOnCompatibleMinorBump(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.1.4")
	runnerPath := writeRunnerScript(t, "1.2.0")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected compatible upgrade to succeed, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.0")
}

func TestReconcileRunnerVersion_UpdatesOnReleaseCandidateBump(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.2.3-rc1")
	runnerPath := writeRunnerScript(t, "1.2.3-rc2")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected release-candidate upgrade to succeed, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.3-rc2")
}

// TestReconcileRunnerVersion_ProceedsAndUpdatesOnDowngrade verifies that an
// older-than-recorded resolved runner is accepted (with a warning) and the
// marker updated to match, rather than refused: there is no older installed
// runner to fall back to instead.
func TestReconcileRunnerVersion_ProceedsAndUpdatesOnDowngrade(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.3.0")
	runnerPath := writeRunnerScript(t, "1.2.9")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected downgrade to proceed with a warning, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.9")
}

func TestReconcileRunnerVersion_KeepsIdenticalVersion(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.2.3")
	runnerPath := writeRunnerScript(t, "1.2.3")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected identical-version reconciliation to succeed, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.3")
}

// TestReconcileRunnerVersion_ProceedsAndUpdatesOnMajorMismatch verifies the
// same policy for a major-version mismatch: proceed with the resolved
// runner and update the marker, since there is no older installed runner to
// fall back to instead.
func TestReconcileRunnerVersion_ProceedsAndUpdatesOnMajorMismatch(t *testing.T) {
	t.Parallel()

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.9.0")
	runnerPath := writeRunnerScript(t, "2.0.0")

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected major-version mismatch to proceed with a warning, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "2.0.0")
}

func TestReconcileRunnerVersion_RejectsInvalidResolvedRunnerVersion(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.2.3")
	runnerPath := filepath.Join(t.TempDir(), "launcher")
	writeExecutableTestFile(t, runnerPath, []byte("#!/bin/sh\nprintf 'invalid-version\\n'\n"))

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)

	// Then
	if err == nil || !strings.Contains(err.Error(), "does not report a valid version") {
		t.Fatalf("expected invalid resolved runner version error, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "1.2.3")
}

func TestReconcileRunnerVersion_ForcedBypassProceedsOnInvalidVersion(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	t.Setenv(forceRunnerReconciliationEnv, "1")
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "1.2.3")
	runnerPath := filepath.Join(t.TempDir(), "launcher")
	writeExecutableTestFile(t, runnerPath, []byte("#!/bin/sh\nprintf 'invalid-version\\n'\n"))

	// When
	err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	// Then
	if err != nil {
		t.Fatalf("expected forced reconciliation to bypass the invalid version, got %v", err)
	}
}

func requirePOSIXRunnerTest(t *testing.T) {
	t.Helper()
	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}
}

func versionedRunner(version, identity string) []byte {
	return []byte("#!/bin/sh\n" +
		"# " + identity + "\n" +
		"if [ \"$1\" = version ]; then\n" +
		"  printf 'v" + version + "\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 2\n")
}
