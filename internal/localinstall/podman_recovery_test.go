// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodmanInstallStart_QuarantinesInterruptedInitialCreate(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	writeTestFile(t, filepath.Join(startConfig.DataDir, initialCreateMarkerName), "incomplete")
	writeTestFile(t, filepath.Join(startConfig.DataDir, "partial-data"), "recoverable")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "running"), "present")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected interrupted create recovery to succeed: %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	expectedPrefix := []string{
		"<podman><container><exists><" + testContainerName + ">",
		"<podman><stop><" + testContainerName + ">",
		"<podman><rm><--force><--ignore><" + testContainerName + ">",
		"<podman><container><exists><" + testContainerName + ">",
	}
	if len(commands) < len(expectedPrefix) {
		t.Fatalf("expected recovery commands, got %#v", commands)
	}
	for index, expected := range expectedPrefix {
		if commands[index] != expected {
			t.Fatalf("expected recovery command %q at %d, got %q", expected, index, commands[index])
		}
	}
	quarantines, globErr := filepath.Glob(startConfig.DataDir + ".failed-*")
	if globErr != nil || len(quarantines) != 1 {
		t.Fatalf("expected one quarantine directory, got %#v, %v", quarantines, globErr)
	}
	content, readErr := os.ReadFile(
		filepath.Join(quarantines[0], "partial-data"),
	) //nolint:gosec // test-owned path
	if readErr != nil || string(content) != "recoverable" {
		t.Fatalf("expected partial data in quarantine, got %q, %v", content, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(startConfig.DataDir, "partial-data")); !os.IsNotExist(
		statErr,
	) {
		t.Fatalf("expected clean live data directory, got %v", statErr)
	}
	runCommand := commands[len(commands)-1]
	if !strings.Contains(runCommand, "<params=maxConnectionsLicenseLimit=20>") {
		t.Fatalf("expected recovered deployment to receive first-start params: %q", runCommand)
	}
}

func TestPodmanInstallStart_RemovesStaleTLSOnlyForFreshDeployment(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, _ := newPodmanInstallFixture(t)
	for _, relativePath := range nanoTLSFiles {
		writeTestFile(t, filepath.Join(startConfig.DataDir, relativePath), "stale")
	}

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected fresh deployment with stale TLS to start: %v", err)
	}
	for _, relativePath := range nanoTLSFiles {
		if _, statErr := os.Stat(filepath.Join(startConfig.DataDir, relativePath)); !os.IsNotExist(
			statErr,
		) {
			t.Fatalf("expected stale TLS file %s to be removed, got %v", relativePath, statErr)
		}
	}
}
