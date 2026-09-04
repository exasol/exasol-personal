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

func newTestRuntimeForReconciliation(t *testing.T) *MacVMRuntime {
	t.Helper()

	testRuntime := NewMacVMRuntime(config.NewDeploymentDir(t.TempDir()), nil)
	if err := os.MkdirAll(testRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}

	return testRuntime
}

func seedVersionMarker(t *testing.T, testRuntime *MacVMRuntime, version string) {
	t.Helper()

	parsed, err := semver.ParseTolerant(version)
	if err != nil {
		t.Fatalf("failed to parse marker version: %v", err)
	}
	if err := writeRunnerVersionMarker(
		testRuntime.paths.RunnerVersionMarkerPath, parsed,
	); err != nil {
		t.Fatalf("failed to seed version marker: %v", err)
	}
}

func assertMarkerVersion(t *testing.T, testRuntime *MacVMRuntime, expected string) {
	t.Helper()

	actual, err := readRunnerVersionMarker(testRuntime.paths.RunnerVersionMarkerPath)
	if err != nil {
		t.Fatalf("failed to read version marker: %v", err)
	}
	if actual.String() != expected {
		t.Fatalf("marker version = %s, want %s", actual, expected)
	}
}

func writeRunnerScript(t *testing.T, version string) string {
	t.Helper()
	requirePOSIXRunnerTest(t)

	path := filepath.Join(t.TempDir(), "launcher")
	writeExecutableTestFile(t, path, versionedRunner(version))

	return path
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionTreatsUnrecordedGuestAsStale(t *testing.T) {
	testRuntime := newTestRuntimeForReconciliation(t)
	runnerPath := writeRunnerScript(t, "2.1.3")

	reconciliation, err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	if err != nil {
		t.Fatalf("expected v2 runner reconciliation: %v", err)
	}
	if reconciliation.Version.String() != "2.1.3" || !reconciliation.Recordable {
		t.Fatalf("reconciliation = %+v, want recordable 2.1.3", reconciliation)
	}
	if !reconciliation.GuestStale {
		t.Fatal("expected an unrecorded guest to be replaced")
	}
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionKeepsGuestForRecordedRunner(t *testing.T) {
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "2.1.3")

	reconciliation, err := testRuntime.reconcileRunnerVersion(
		context.Background(), writeRunnerScript(t, "2.1.3"),
	)
	if err != nil {
		t.Fatalf("expected v2 runner reconciliation: %v", err)
	}
	if reconciliation.GuestStale {
		t.Fatal("expected a guest matching the resolved runner to be kept")
	}
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionUpdatesCompatibleUpgradeAndDowngrade(t *testing.T) {
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "2.1.0")
	upgrade, err := testRuntime.reconcileRunnerVersion(
		context.Background(), writeRunnerScript(t, "2.2.0"),
	)
	if err != nil {
		t.Fatalf("expected compatible upgrade: %v", err)
	}
	if upgrade.Version.String() != "2.2.0" || !upgrade.GuestStale {
		t.Fatalf("upgrade reconciliation = %+v, want stale 2.2.0", upgrade)
	}

	downgrade, err := testRuntime.reconcileRunnerVersion(
		context.Background(), writeRunnerScript(t, "2.1.9"),
	)
	if err != nil {
		t.Fatalf("expected compatible downgrade: %v", err)
	}
	if downgrade.Version.String() != "2.1.9" || !downgrade.GuestStale {
		t.Fatalf("downgrade reconciliation = %+v, want stale 2.1.9", downgrade)
	}
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionAcceptsNewerContractMajor(t *testing.T) {
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "2.1.0")

	reconciliation, err := testRuntime.reconcileRunnerVersion(
		context.Background(), writeRunnerScript(t, "3.0.0"),
	)
	if err != nil {
		t.Fatalf("expected newer runner contract: %v", err)
	}
	if reconciliation.Version.String() != "3.0.0" || !reconciliation.GuestStale {
		t.Fatalf("reconciliation = %+v, want stale 3.0.0", reconciliation)
	}
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionRejectsLegacyV1BeforeUpdatingMarker(t *testing.T) {
	testRuntime := newTestRuntimeForReconciliation(t)
	seedVersionMarker(t, testRuntime, "2.1.0")

	_, err := testRuntime.reconcileRunnerVersion(
		context.Background(), writeRunnerScript(t, "1.9.9"),
	)
	if err == nil || !strings.Contains(err.Error(), "legacy application-owning contract") {
		t.Fatalf("expected legacy runner rejection, got %v", err)
	}
	assertMarkerVersion(t, testRuntime, "2.1.0")
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionRejectsInvalidVersion(t *testing.T) {
	testRuntime := newTestRuntimeForReconciliation(t)
	runnerPath := filepath.Join(t.TempDir(), "launcher")
	writeExecutableTestFile(t, runnerPath, []byte("#!/bin/sh\nprintf 'invalid-version\\n'\n"))

	_, err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	if err == nil || !strings.Contains(err.Error(), "does not report a valid version") {
		t.Fatalf("expected invalid version error, got %v", err)
	}
}

//nolint:paralleltest // tests set process environment and fork executable fixtures.
func TestReconcileRunnerVersionForcedBypassAllowsUnversionedDevelopmentRunner(t *testing.T) {
	t.Setenv(forceRunnerReconciliationEnv, "1")
	testRuntime := newTestRuntimeForReconciliation(t)
	runnerPath := filepath.Join(t.TempDir(), "launcher")
	writeExecutableTestFile(t, runnerPath, []byte("#!/bin/sh\nprintf 'dev\\n'\n"))

	reconciliation, err := testRuntime.reconcileRunnerVersion(context.Background(), runnerPath)
	if err != nil {
		t.Fatalf("expected forced development runner: %v", err)
	}
	if reconciliation.Recordable || reconciliation.GuestStale {
		t.Fatalf("reconciliation = %+v, want an unrecordable version", reconciliation)
	}
}

func requirePOSIXRunnerTest(t *testing.T) {
	t.Helper()
	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}
}

func versionedRunner(version string) []byte {
	return []byte("#!/bin/sh\n" +
		"if [ \"$1\" = version ]; then\n" +
		"  printf 'v" + version + "\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 2\n")
}
