// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestReconcileRunner_InstallsMissingRunner(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	embedded := versionedRunner("1.2.3", "embedded")

	// When
	err := reconcileRunner(context.Background(), targetPath, embedded)
	// Then
	assertRunnerContent(t, targetPath, embedded)
	if err != nil {
		t.Fatalf("expected missing runner installation to succeed, got %v", err)
	}
}

func TestReconcileRunner_UpgradesUnversionedRunner(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	writeExecutableTestFile(t, targetPath, []byte("#!/bin/sh\nexit 2\n"))
	embedded := versionedRunner("1.2.3", "embedded")

	// When
	err := reconcileRunner(context.Background(), targetPath, embedded)
	// Then
	if err != nil {
		t.Fatalf("expected unversioned runner upgrade to succeed, got %v", err)
	}
	assertRunnerContent(t, targetPath, embedded)
}

func TestReconcileRunner_UpgradesNewerMinorRunner(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	writeExecutableTestFile(t, targetPath, versionedRunner("1.1.4", "installed"))
	embedded := versionedRunner("1.2.0", "embedded")

	// When
	err := reconcileRunner(context.Background(), targetPath, embedded)
	// Then
	if err != nil {
		t.Fatalf("expected compatible runner upgrade to succeed, got %v", err)
	}
	assertRunnerContent(t, targetPath, embedded)
}

func TestReconcileRunner_UpgradesNewerReleaseCandidate(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	writeExecutableTestFile(t, targetPath, versionedRunner("1.2.3-rc1", "installed"))
	embedded := versionedRunner("1.2.3-rc2", "embedded")

	// When
	err := reconcileRunner(context.Background(), targetPath, embedded)
	// Then
	if err != nil {
		t.Fatalf("expected release-candidate runner upgrade to succeed, got %v", err)
	}
	assertRunnerContent(t, targetPath, embedded)
}

func TestReconcileRunner_DoesNotDowngradeRunner(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	installed := versionedRunner("1.3.0", "installed")
	writeExecutableTestFile(t, targetPath, installed)

	// When
	err := reconcileRunner(context.Background(), targetPath, versionedRunner("1.2.9", "embedded"))
	// Then
	if err != nil {
		t.Fatalf("expected runner downgrade to be skipped, got %v", err)
	}
	assertRunnerContent(t, targetPath, installed)
}

func TestReconcileRunner_KeepsIdenticalRunner(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	embedded := versionedRunner("1.2.3", "same")
	writeExecutableTestFile(t, targetPath, embedded)

	// When
	err := reconcileRunner(context.Background(), targetPath, embedded)
	// Then
	if err != nil {
		t.Fatalf("expected identical runner reconciliation to succeed, got %v", err)
	}
	assertRunnerContent(t, targetPath, embedded)
}

func TestReconcileRunner_RepairsDifferentRunnerWithSameVersion(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	writeExecutableTestFile(t, targetPath, versionedRunner("1.2.3", "installed"))
	embedded := versionedRunner("1.2.3", "embedded")

	// When
	err := reconcileRunner(context.Background(), targetPath, embedded)
	// Then
	if err != nil {
		t.Fatalf("expected same-version runner repair to succeed, got %v", err)
	}
	assertRunnerContent(t, targetPath, embedded)
}

func TestReconcileRunner_PreservesRunnerAcrossMajorVersions(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	installed := versionedRunner("1.9.0", "installed")
	writeExecutableTestFile(t, targetPath, installed)

	// When
	err := reconcileRunner(context.Background(), targetPath, versionedRunner("2.0.0", "embedded"))
	// Then
	if err != nil {
		t.Fatalf("expected major runner update to be skipped, got %v", err)
	}
	assertRunnerContent(t, targetPath, installed)
}

func TestReconcileRunner_RejectsInvalidEmbeddedRunnerWithoutChangingInstalled(t *testing.T) {
	t.Parallel()
	requirePOSIXRunnerTest(t)

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	installed := versionedRunner("1.2.3", "installed")
	writeExecutableTestFile(t, targetPath, installed)

	// When
	err := reconcileRunner(
		context.Background(),
		targetPath,
		[]byte("#!/bin/sh\nprintf 'invalid-version\\n'\n"),
	)
	// Then
	if err == nil ||
		!strings.Contains(err.Error(), "embedded local runner does not report a valid version") {
		t.Fatalf("expected invalid embedded runner error, got %v", err)
	}
	assertRunnerContent(t, targetPath, installed)
}

func TestReconcileRunnerExecutable_InternalOverrideForcesUnversionedRunnerUpdate(t *testing.T) {
	requirePOSIXRunnerTest(t)

	// Given
	t.Setenv(forceRunnerReconciliationEnv, "1")
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := newRuntime(deployment, Config{})
	embedded := []byte("#!/bin/sh\n# embedded\nexit 1\n")
	localRuntime.embeddedRunner = embedded
	localRuntime.embeddedRunnerExists = true
	if err := os.MkdirAll(localRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runner directory: %v", err)
	}
	writeExecutableTestFile(
		t,
		localRuntime.paths.RunnerPath,
		[]byte("#!/bin/sh\n# installed\nexit 2\n"),
	)

	// When
	err := localRuntime.reconcileRunnerExecutable(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected internal override to force reconciliation, got %v", err)
	}
	assertRunnerContent(t, localRuntime.paths.RunnerPath, embedded)
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

func assertRunnerContent(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected runner %s to exist", path)
		}
		t.Fatalf("failed to read runner %s: %v", path, err)
	}
	if string(actual) != string(expected) {
		t.Fatalf("expected runner content %q, got %q", string(expected), string(actual))
	}
}
