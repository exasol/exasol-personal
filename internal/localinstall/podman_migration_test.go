// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodmanInstallStart_MigratesLegacyOverlayDataAtomically(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newLegacyOverlayFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "exasol.conf"), "migrated")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "storage", "data"), "payload")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected legacy overlay migration to succeed: %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	expectedPrefix := []string{
		"<podman><container><exists><" + testContainerName + ">",
		"<podman><container><inspect><--format><{{.State.Running}}><" + testContainerName + ">",
		"<podman><container><inspect><--format><{{json .Mounts}}><" + testContainerName + ">",
		"<podman><stop><" + testContainerName + ">",
	}
	if len(commands) < len(expectedPrefix)+2 {
		t.Fatalf("expected migration commands, got %#v", commands)
	}
	for index, expected := range expectedPrefix {
		if commands[index] != expected {
			t.Fatalf("expected command %q at %d, got %q", expected, index, commands[index])
		}
	}
	if !strings.HasPrefix(commands[4], "<podman><cp><"+testContainerName+":/exa/.><") {
		t.Fatalf("expected staged Podman copy, got %q", commands[4])
	}
	if commands[5] != "<podman><rm><"+testContainerName+">" {
		t.Fatalf("expected old container removal after copy, got %q", commands[5])
	}
	content, readErr := os.ReadFile(
		filepath.Join(startConfig.DataDir, "storage", "data"),
	) //nolint:gosec // test-owned path
	if readErr != nil || string(content) != "payload" {
		t.Fatalf("expected migrated data, got %q, %v", content, readErr)
	}
	if strings.Contains(commands[len(commands)-1], "<params=") {
		t.Fatalf(
			"migrated initialized data must not receive first-start params: %q",
			commands[len(commands)-1],
		)
	}
	assertNoMigrationStagingDirs(t, startConfig.DataDir)
}

func TestPodmanInstallStart_AdoptsAndMigratesLegacyNamedVolumeContainer(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.LegacyContainerNames = []string{"exasol-local-db"}
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "legacy-name"), "exasol-local-db")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "exasol.conf"), "migrated")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "storage", "data"), "payload")

	if err := install.Start(context.Background(), nil, nil, startConfig); err != nil {
		t.Fatalf("expected legacy named-volume migration to succeed: %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	wantPrefix := []string{
		"<podman><container><exists><" + testContainerName + ">",
		"<podman><container><exists><exasol-local-db>",
		"<podman><rename><exasol-local-db><" + testContainerName + ">",
		"<podman><container><exists><" + testContainerName + ">",
		"<podman><container><inspect><--format><{{.State.Running}}><" + testContainerName + ">",
		"<podman><container><inspect><--format><{{json .Mounts}}><" + testContainerName + ">",
		"<podman><stop><" + testContainerName + ">",
	}
	if len(commands) < len(wantPrefix)+2 {
		t.Fatalf("expected adoption and migration commands, got %#v", commands)
	}
	for index, expected := range wantPrefix {
		if commands[index] != expected {
			t.Fatalf("command %d = %q, want %q", index, commands[index], expected)
		}
	}
	content, err := os.ReadFile(filepath.Join(startConfig.DataDir, "storage", "data"))
	if err != nil || string(content) != "payload" {
		t.Fatalf("expected migrated named-volume data, got %q, %v", content, err)
	}
}

func TestPodmanInstallStart_AdoptsLegacyNamedContainerUsingPersistentData(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.LegacyContainerNames = []string{"exasol-local-db"}
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "legacy-name"), "exasol-local-db")
	writeTestFile(t, filepath.Join(startConfig.DataDir, "storage", "data"), "preserved")
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "mounts-output"),
		fmt.Sprintf(
			`[{"Type":"bind","Source":%q,"Destination":"/exa"}]`, startConfig.DataDir,
		),
	)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected legacy container adoption to preserve mounted data: %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	for _, command := range commands {
		if strings.HasPrefix(command, "<podman><cp>") ||
			command == "<podman><stop><"+testContainerName+">" {
			t.Fatalf("expected adoption without migration, got command %q", command)
		}
	}
	content, readErr := os.ReadFile(
		filepath.Join(startConfig.DataDir, "storage", "data"),
	) //nolint:gosec // test-owned path
	if readErr != nil || string(content) != "preserved" {
		t.Fatalf("expected persistent data to remain untouched, got %q, %v", content, readErr)
	}
}

func TestPodmanInstallStart_RefusesToOverwritePopulatedMigrationDestination(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newLegacyOverlayFixture(t)
	writeTestFile(t, filepath.Join(startConfig.DataDir, "existing-data"), "keep")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)

	// Then
	if err == nil ||
		!strings.Contains(err.Error(), "refusing to overwrite populated persistent Nano data") {
		t.Fatalf("expected populated destination refusal, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	if len(commands) != 3 {
		t.Fatalf("expected inspection without stop or copy, got %#v", commands)
	}
	content, readErr := os.ReadFile(
		filepath.Join(startConfig.DataDir, "existing-data"),
	) //nolint:gosec // test-owned path
	if readErr != nil || string(content) != "keep" {
		t.Fatalf("expected destination data to remain untouched, got %q, %v", content, readErr)
	}
	assertLegacyContainerRetained(t, fixture)
}

func TestPodmanInstallStart_RetainsLegacyContainerWhenOverlayCopyFails(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newLegacyOverlayFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), "cp")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)

	// Then
	if err == nil || !strings.Contains(err.Error(), "the stopped container was retained") ||
		!strings.Contains(err.Error(), "podman cp") {
		t.Fatalf("expected actionable copy failure, got %v", err)
	}
	assertLegacyContainerRetained(t, fixture)
	assertNoMigrationStagingDirs(t, startConfig.DataDir)
	for _, command := range readCommandLog(t, fixture.logPath) {
		if command == "<podman><rm><"+testContainerName+">" {
			t.Fatalf("legacy container was removed after copy failure: %q", command)
		}
	}
}

func TestPodmanInstallStart_RetainsStagedCopyWhenDestinationChanges(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newLegacyOverlayFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "exasol.conf"), "copied")
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "populate-destination-during-cp"),
		startConfig.DataDir,
	)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)

	// Then
	if err == nil || !strings.Contains(err.Error(), "became populated during migration") ||
		!strings.Contains(err.Error(), "staging directory were retained") {
		t.Fatalf("expected recoverable atomic-install refusal, got %v", err)
	}
	assertLegacyContainerRetained(t, fixture)
	raced, readErr := os.ReadFile(
		filepath.Join(startConfig.DataDir, "raced-data"),
	) //nolint:gosec // test-owned path
	if readErr != nil || string(raced) != "raced\n" {
		t.Fatalf("expected raced destination data to remain, got %q, %v", raced, readErr)
	}
	staging := migrationStagingDirs(t, startConfig.DataDir)
	if len(staging) != 1 {
		t.Fatalf("expected recoverable staged copy, got %#v", staging)
	}
	if _, statErr := os.Stat(filepath.Join(staging[0], "exasol.conf")); statErr != nil {
		t.Fatalf("expected copied data in retained staging directory: %v", statErr)
	}
}

func TestPodmanInstallStart_ReportsLegacyRemovalFailureAfterDataInstall(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newLegacyOverlayFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "exasol.conf"), "migrated")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), "rm")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)

	// Then
	if err == nil || !strings.Contains(err.Error(), "data is installed") ||
		!strings.Contains(err.Error(), "verify the data and remove that container") {
		t.Fatalf("expected actionable legacy removal failure, got %v", err)
	}
	assertLegacyContainerRetained(t, fixture)
	if _, statErr := os.Stat(filepath.Join(startConfig.DataDir, "exasol.conf")); statErr != nil {
		t.Fatalf("expected atomically installed data to remain: %v", statErr)
	}
	assertNoMigrationStagingDirs(t, startConfig.DataDir)
}

func newLegacyOverlayFixture(t *testing.T) (*PodmanInstall, StartConfig, podmanInstallFixture) {
	t.Helper()

	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "running"), "present")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "mounts-output"), "[]\n")

	return install, startConfig, fixture
}

func assertLegacyContainerRetained(t *testing.T, fixture podmanInstallFixture) {
	t.Helper()
	for _, stateFile := range []string{"running", "existing"} {
		if _, err := os.Stat(filepath.Join(fixture.scenarioDir, stateFile)); err == nil {
			return
		}
	}
	t.Fatal("expected legacy container to be retained")
}

func assertNoMigrationStagingDirs(t *testing.T, dataDir string) {
	t.Helper()
	if staging := migrationStagingDirs(t, dataDir); len(staging) != 0 {
		t.Fatalf("expected no migration staging directories, got %#v", staging)
	}
}

func migrationStagingDirs(t *testing.T, dataDir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(
		filepath.Dir(dataDir), "."+filepath.Base(dataDir)+".overlay-migration-*",
	))
	if err != nil {
		t.Fatalf("failed to inspect migration staging directories: %v", err)
	}

	return paths
}

// Regression: on Windows the data directory is a host path bind-mounted into
// the Podman machine, and Podman reports the source as the machine sees it
// (/mnt/c/...). Comparing that against the Windows data directory always
// failed, so every surviving container looked like a legacy overlay one and
// the migration refused to overwrite the deployment's own data.
func TestPodmanInstallStart_TreatsTranslatedBindMountAsPersistent(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given a running container whose /exa bind mount is reported with a
	// machine-side path that cannot be compared to the host data directory,
	// and a data directory that already holds data
	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "running"), testContainerName+"\n")
	writeTestFile(t, filepath.Join(startConfig.DataDir, "storage", "data"), "preserved")
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "mounts-output"),
		`[{"Type":"bind",`+
			`"Source":"/mnt/c/Users/someone/deployment/local/runtime/exa",`+
			`"Destination":"/exa"}]`,
	)
	install.resolveImage = nil

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then the start succeeds rather than refusing to overwrite the data
	if err != nil {
		t.Fatalf("expected a translated bind mount to count as persistent: %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	for _, command := range commands {
		if strings.Contains(command, "<cp>") {
			t.Fatalf("no migration copy should occur, got %#v", commands)
		}
	}
	// And the existing data is untouched
	content, readErr := os.ReadFile(
		filepath.Join(startConfig.DataDir, "storage", "data"),
	) //nolint:gosec // test-owned path
	if readErr != nil || string(content) != "preserved" {
		t.Fatalf("expected the existing data to survive, got %q, %v", content, readErr)
	}
}

// A non-bind mount at /exa is not deployment-owned persistent storage, so a
// volume-backed container must still be migrated.
func TestPodmanInstallStart_MigratesVolumeBackedExaData(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	install, startConfig, fixture := newLegacyOverlayFixture(t)
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "mounts-output"),
		`[{"Type":"volume","Source":"exa-data","Destination":"/exa"}]`,
	)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "cp-source", "exasol.conf"), "migrated")

	err := install.Start(context.Background(), nil, nil, startConfig)
	if err != nil {
		t.Fatalf("expected volume-backed data to migrate: %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	migrated := false
	for _, command := range commands {
		if strings.Contains(command, "<cp><"+testContainerName+":/exa/.><") {
			migrated = true
		}
	}
	if !migrated {
		t.Fatalf("expected a migration copy for volume-backed data, got %#v", commands)
	}
}
