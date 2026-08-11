// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodmanInstallStart_MaterializesAndMountsAvailableSLCs(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	const (
		existingImage = "docker.io/exasol/script-language-container:existing"
		pulledImage   = "docker.io/exasol/script-language-container:pulled"
		importedImage = "localhost/custom:imported"
		missingImage  = "localhost/custom:missing"
		failedImage   = "localhost/custom:failed"
	)
	startConfig.SLCs = []SLCConfig{
		{Image: existingImage, Target: "/exa/slc/existing"},
		{Image: pulledImage, Target: "/exa/slc/pulled"},
		{Image: importedImage, Target: "/exa/slc/imported", Package: "imported.tar.gz"},
		{Image: missingImage, Target: "/exa/slc/missing", Package: "missing.tar.gz"},
		{Image: failedImage, Target: "/exa/slc/failed", Package: "failed.tar.gz"},
	}
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "images"), existingImage+"\n")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail-import-image"), failedImage)
	writeTestFile(t, filepath.Join(fixture.slcStagingDir, "imported.tar.gz"), "image archive")
	writeTestFile(t, filepath.Join(fixture.slcStagingDir, "failed.tar.gz"), "invalid archive")
	writeTestFile(t, fixture.slcStatusPath, `{"slc":[{"image":"stale","state":"present"}]}`)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected custom import failure to leave Nano startable, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	joinedCommands := strings.Join(commands, "\n")
	expectedImport := "<podman><import><--change><LABEL " + slcImportLabel + "><" +
		filepath.Join(fixture.slcStagingDir, "imported.tar.gz") + "><" + importedImage + ">"
	if !strings.Contains(joinedCommands, expectedImport) {
		t.Fatalf("expected labeled custom import %q in %#v", expectedImport, commands)
	}
	if !strings.Contains(joinedCommands, "<podman><pull><"+pulledImage+">") {
		t.Fatalf("expected official image pull, got %#v", commands)
	}
	runCommand := commands[len(commands)-1]
	for _, expectedMount := range []string{
		"<--mount><type=image,source=" + existingImage + ",destination=/exa/slc/existing>",
		"<--mount><type=image,source=" + pulledImage + ",destination=/exa/slc/pulled>",
		"<--mount><type=image,source=" + importedImage + ",destination=/exa/slc/imported>",
	} {
		if !strings.Contains(runCommand, expectedMount) {
			t.Fatalf("expected mount %q in %q", expectedMount, runCommand)
		}
	}
	if strings.Contains(runCommand, missingImage) || strings.Contains(runCommand, failedImage) {
		t.Fatalf("unavailable custom SLCs must not be mounted: %q", runCommand)
	}

	statusData, readErr := os.ReadFile(fixture.slcStatusPath) //nolint:gosec // test-owned path
	if readErr != nil {
		t.Fatalf("expected SLC status report: %v", readErr)
	}
	var report slcStatusReport
	if err := json.Unmarshal(statusData, &report); err != nil {
		t.Fatalf("expected valid SLC status JSON, got %q: %v", statusData, err)
	}
	expectedStates := []string{
		slcStatusPresent,
		slcStatusPulled,
		slcStatusImported,
		slcStatusMissing,
		slcStatusImportFailed,
	}
	if len(report.SLC) != len(expectedStates) {
		t.Fatalf("unexpected SLC status report: %#v", report)
	}
	for index, expectedState := range expectedStates {
		if report.SLC[index].State != expectedState {
			t.Fatalf("expected state %q at %d, got %#v", expectedState, index, report.SLC)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(fixture.slcStatusPath))
	if err != nil {
		t.Fatalf("failed to inspect status directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".slc-status-") {
			t.Fatalf("temporary status report was not cleaned up: %s", entry.Name())
		}
	}
}

func TestPodmanInstallStart_OfficialSLCPullFailureIsFatal(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.SLCs = []SLCConfig{{
		Image:  "docker.io/exasol/script-language-container:missing",
		Target: "/exa/slc/missing",
	}}
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail"), "pull")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)

	// Then
	if err == nil || !strings.Contains(err.Error(), "failed to pull SLC image") {
		t.Fatalf("expected fatal official SLC pull failure, got %v", err)
	}
	for _, command := range readCommandLog(t, fixture.logPath) {
		if strings.Contains(command, "<podman><run>") {
			t.Fatalf("Nano must not start after requested official SLC pull failure: %q", command)
		}
	}
}

func TestPodmanInstallStart_RewritesAuthoritativeEmptySLCStatus(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.SLCs = []SLCConfig{}
	writeTestFile(t, fixture.slcStatusPath, `{"slc":[{"image":"stale","state":"present"}]}`)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected empty SLC startup to succeed: %v", err)
	}
	statusData, readErr := os.ReadFile(fixture.slcStatusPath) //nolint:gosec // test-owned path
	if readErr != nil {
		t.Fatalf("expected empty SLC status report: %v", readErr)
	}
	if strings.TrimSpace(string(statusData)) != `{"slc":[]}` {
		t.Fatalf("expected authoritative empty report, got %q", statusData)
	}
}

func TestNormalizeSLCImageRef(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"docker.io/exasol/script-language-container:python": "exasol/script-language-container:python",
		"localhost/custom:sha256":                           "custom:sha256",
		"exasol/script-language-container":                  "exasol/script-language-container:latest",
		"registry.example.test:5000/custom/image":           "registry.example.test:5000/custom/image:latest",
	}
	for input, expected := range tests {
		if actual := normalizeSLCImageRef(input); actual != expected {
			t.Errorf("normalizeSLCImageRef(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestPodmanInstallStart_PrunesOnlyUnreferencedManagedSLCImages(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.SLCs = []SLCConfig{}
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "images-output"), strings.Join([]string{
		"docker.io/exasol/script-language-container:old",
		"docker.io/exasol/script-language-container:<none>",
		"docker.io/acme/exasol/script-language-container:unrelated",
		"docker.io/exasol/script-language-container-helper:unrelated",
	}, "\n")+"\n")
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "labeled-images-output"),
		strings.Join([]string{
			"localhost/custom:old",
			"localhost/custom:<none>",
		}, "\n")+"\n",
	)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected pruning to remain nonfatal: %v", err)
	}
	commands := strings.Join(readCommandLog(t, fixture.logPath), "\n")
	for _, expected := range []string{
		"<podman><rmi><docker.io/exasol/script-language-container:old>",
		"<podman><rmi><localhost/custom:old>",
	} {
		if !strings.Contains(commands, expected) {
			t.Fatalf("expected managed image removal %q in %s", expected, commands)
		}
	}
	for _, protected := range []string{
		"script-language-container:<none>",
		"acme/exasol/script-language-container:unrelated",
		"script-language-container-helper:unrelated",
		"custom:<none>",
	} {
		if strings.Contains(commands, "<podman><rmi><docker.io/"+protected+">") ||
			strings.Contains(commands, "<podman><rmi><localhost/"+protected+">") {
			t.Fatalf("protected image %q was pruned: %s", protected, commands)
		}
	}
}

func TestPodmanInstallStart_KeepsNormalizedReferencedSLCImages(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	const (
		officialDesired = "docker.io/exasol/script-language-container"
		customDesired   = "custom:kept"
	)
	startConfig.SLCs = []SLCConfig{
		{Image: officialDesired, Target: "/exa/slc/official"},
		{Image: customDesired, Target: "/exa/slc/custom", Package: "custom.tar.gz"},
	}
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "images"),
		officialDesired+"\n"+customDesired+"\n",
	)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "images-output"), strings.Join([]string{
		"exasol/script-language-container:latest",
		"exasol/script-language-container:stale",
	}, "\n")+"\n")
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "labeled-images-output"),
		strings.Join([]string{
			"localhost/custom:kept",
			"localhost/custom:stale",
		}, "\n")+"\n",
	)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected referenced image pruning to succeed: %v", err)
	}
	commands := strings.Join(readCommandLog(t, fixture.logPath), "\n")
	if strings.Contains(commands, "<podman><rmi><exasol/script-language-container:latest>") ||
		strings.Contains(commands, "<podman><rmi><localhost/custom:kept>") {
		t.Fatalf("normalized referenced images were pruned: %s", commands)
	}
	for _, stale := range []string{
		"<podman><rmi><exasol/script-language-container:stale>",
		"<podman><rmi><localhost/custom:stale>",
	} {
		if !strings.Contains(commands, stale) {
			t.Fatalf("expected stale image removal %q in %s", stale, commands)
		}
	}
}

func TestPodmanInstallStart_SLCPruningFailuresAreNonfatal(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newPodmanInstallFixture(t)
	startConfig.SLCs = []SLCConfig{}
	const staleImage = "docker.io/exasol/script-language-container:stale"
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "images-output"), staleImage+"\n")
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "fail-rmi-image"), staleImage)
	var outErr strings.Builder

	// When
	err := install.Start(context.Background(), nil, &outErr, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected failed pruning to remain nonfatal: %v", err)
	}
	if !strings.Contains(outErr.String(), "failed to remove unreferenced SLC image") {
		t.Fatalf("expected pruning warning, got %q", outErr.String())
	}
}
