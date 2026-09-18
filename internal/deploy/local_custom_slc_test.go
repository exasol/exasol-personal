// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestPromoteCustomSLCPackageMovesTheContainerIntoPlace(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	content := []byte("container-bytes")
	tempPath := writeStagingFile(t, deployment, content)

	// When
	err := promoteCustomSLCPackage(deployment, tempPath, "custom-mypy3-abc.tar.gz")
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
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected the temporary file moved away, got %v", err)
	}
}

func TestPromoteCustomSLCPackageReplacesAnExistingPackage(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	const name = "custom-mypy3-abc.tar.gz"
	if err := promoteCustomSLCPackage(
		deployment, writeStagingFile(t, deployment, []byte("first")), name,
	); err != nil {
		t.Fatal(err)
	}

	// When
	err := promoteCustomSLCPackage(
		deployment, writeStagingFile(t, deployment, []byte("second")), name,
	)
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

	// When: opening a staging file for a new container.
	temp, err := newCustomSLCStagingFile(deployment)
	if err != nil {
		t.Fatal(err)
	}
	defer temp.Close()

	// Then: the leftover is gone and the fresh staging file is in its place.
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("expected the stale partial download removed, got %v", err)
	}
	if _, err := os.Stat(temp.Name()); err != nil {
		t.Fatalf("expected the fresh staging file, got %v", err)
	}
}

func writeStagingFile(t *testing.T, deployment config.DeploymentDir, content []byte) string {
	t.Helper()

	temp, err := newCustomSLCStagingFile(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := temp.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}

	return temp.Name()
}
