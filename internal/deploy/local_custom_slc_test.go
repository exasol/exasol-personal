// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestStageCustomSLCPackageWritesTheContainer(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	content := []byte("container-bytes")

	// When
	err := stageCustomSLCPackage(deployment, "custom-mypy3-abc.tar.gz", bytes.NewReader(content))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	staged := filepath.Join(customSLCStagingDir(deployment), "custom-mypy3-abc.tar.gz")
	got, err := os.ReadFile(staged) //nolint:gosec // test-owned path
	if err != nil {
		t.Fatalf("expected the package staged at %s: %v", staged, err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("staged content mismatch: %q", got)
	}
}

func TestStageCustomSLCPackageIsIdempotent(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	const name = "custom-mypy3-abc.tar.gz"
	if err := stageCustomSLCPackage(deployment, name, strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}

	// When
	err := stageCustomSLCPackage(deployment, name, strings.NewReader("second"))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile( //nolint:gosec // test-owned path
		filepath.Join(customSLCStagingDir(deployment), name),
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("expected the package replaced, got %q", got)
	}
}

func TestStageCustomSLCPackageLeavesNothingOnFailure(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// When
	err := stageCustomSLCPackage(deployment, "custom-mypy3-abc.tar.gz", failingReader{})
	// Then
	if err == nil {
		t.Fatal("expected a staging error")
	}

	entries, readErr := os.ReadDir(customSLCStagingDir(deployment))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no staged files, got %d", len(entries))
	}
}

func TestRemoveCustomSLCPackageToleratesAbsence(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())

	// When / Then
	if err := removeCustomSLCPackage(deployment, "missing.tar.gz"); err != nil {
		t.Fatalf("removing an absent package must succeed, got %v", err)
	}
	if err := removeCustomSLCPackage(deployment, ""); err != nil {
		t.Fatalf("an empty package name must be a no-op, got %v", err)
	}
}

func TestReadSLCImageStatesTreatsAMissingReportAsEmpty(t *testing.T) {
	t.Parallel()

	// When
	states, err := readSLCImageStates(config.NewDeploymentDir(t.TempDir()))
	// Then
	if err != nil {
		t.Fatalf("a missing report must not be an error, got %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected no states, got %v", states)
	}
}

func TestReadSLCImageStatesParsesTheReport(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	writeSLCStatusReport(t, deployment, `{"slc":[
		{"image":"a:1","state":"imported"},
		{"image":"b:1","state":"package-missing"}
	]}`)

	// When
	states, err := readSLCImageStates(deployment)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if states["a:1"] != "imported" || states["b:1"] != "package-missing" {
		t.Fatalf("unexpected states: %v", states)
	}
}

func TestCustomSLCImageAvailable(t *testing.T) {
	t.Parallel()

	// Given
	states := map[string]string{
		"imported:1": slcStateImported,
		"present:1":  slcStatePresent,
		"pulled:1":   slcStatePulled,
		"missing:1":  "package-missing",
		"failed:1":   "import-failed",
	}

	// When / Then
	for _, image := range []string{"imported:1", "present:1", "pulled:1"} {
		if !customSLCImageAvailable(states, image) {
			t.Fatalf("%s must count as available", image)
		}
	}
	for _, image := range []string{"missing:1", "failed:1", "unreported:1"} {
		if customSLCImageAvailable(states, image) {
			t.Fatalf("%s must not count as available", image)
		}
	}
}

func writeSLCStatusReport(t *testing.T, deployment config.DeploymentDir, content string) {
	t.Helper()

	path := customSLCStatusPath(deployment)
	if err := os.MkdirAll(filepath.Dir(path), slcStagingDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func TestStagingDiscardsStalePartialDownloads(t *testing.T) {
	t.Parallel()

	// Given: a leftover partial download from a killed process.
	deployment := config.NewDeploymentDir(t.TempDir())
	dir := customSLCStagingDir(deployment)
	if err := os.MkdirAll(dir, slcStagingDirMode); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "incoming-123456.part")
	if err := os.WriteFile(stale, []byte("half a container"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: staging anything at all.
	err := stageCustomSLCPackage(deployment, "custom-x-abc.tar.gz", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}

	// Then: the leftover is gone and the real package is in place.
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("expected the stale partial download removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom-x-abc.tar.gz")); err != nil {
		t.Fatalf("expected the staged package, got %v", err)
	}
}
