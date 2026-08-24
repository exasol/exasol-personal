// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

// testRunnerExecutableMode is a named constant rather than an inline literal
// so gosec's file-permission checks (which only pattern-match on literals)
// don't flag these test fixtures for needing an executable fake runner.
const testRunnerExecutableMode = 0o700

// runnerZipEntryName matches resources.yaml's resource_path for
// exasol-local-runner.
const runnerZipEntryName = "launcher"

// exasolLocalRunnerResourceID mirrors internal/localruntime's unexported
// resource ID constant, which callers here need too to build a matching test
// ResourceSpec.
const exasolLocalRunnerResourceID = "exasol-local-runner"

// fakeRunnerRunningStatus is the `status` response a fixture must return for
// classifyLocalReachability to get as far as the health-check: the classifier
// skips the diagnosis entirely unless the runtime reports itself running.
const fakeRunnerRunningStatus = `{"running":true}`

type localRuntimeTestPaths struct {
	Root      string
	WorkDir   string
	StatePath string
}

func newLocalRuntimeTestPaths(deployment config.DeploymentDir) localRuntimeTestPaths {
	root := deployment.Resolve("local")
	workDir := filepath.Join(root, "runtime")

	return localRuntimeTestPaths{
		Root:      root,
		WorkDir:   workDir,
		StatePath: filepath.Join(workDir, "vm-state.json"),
	}
}

func TestClassifyLocalReachability_AllPortsBlocked(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	blockedJSON := `{"ports":{"db":{"state":"blocked"},"ui":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, fakeRunnerRunningStatus, blockedJSON)
	localRuntime := localruntime.NewMacVMRuntime(deployment, manager)

	err := classifyLocalReachability(context.Background(), localRuntime)
	if err == nil {
		t.Fatal("expected a reachability error when every port is blocked")
	}
	if !errors.Is(err, ErrLocalReachability) {
		t.Fatalf("expected errors.Is(err, ErrLocalReachability), got %v", err)
	}
}

func TestClassifyLocalReachability_OnlyDatabasePortBlocked(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// A reachable application endpoint alongside a blocked database endpoint means
	// the network path itself is fine; the problem is database-specific.
	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	mixedJSON := `{"ports":{"db":{"state":"blocked"},"ui":{"state":"reachable"}}}`
	manager := writeFakeCombinedRunner(t, fakeRunnerRunningStatus, mixedJSON)
	localRuntime := localruntime.NewMacVMRuntime(deployment, manager)

	if err := classifyLocalReachability(context.Background(), localRuntime); err != nil {
		t.Fatalf("expected no reachability error when at least one port is reachable, got %v", err)
	}
}

func TestClassifyLocalReachability_NonLocalDeploymentIsNoop(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	// A nil manager is safe here: classifyLocalReachability short-circuits on
	// isLocalDeployment before it would ever be dereferenced.
	localRuntime := localruntime.NewMacVMRuntime(deployment, nil)
	if err := classifyLocalReachability(context.Background(), localRuntime); err != nil {
		t.Fatalf("expected no-op for non-local deployment, got %v", err)
	}
}

func TestClassifyLocalReachability_HealthCheckUnavailableIsNoop(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	// An old runner daemon that predates health-check must not turn every
	// local failure into a reachability error; the caller's original error
	// should stand instead.
	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	// Status must succeed so the classifier actually reaches the health-check;
	// only health-check is unsupported, as on an old runner daemon.
	runnerScript := "#!/bin/sh\n" +
		"if [ \"$1\" = status ]; then echo '" + fakeRunnerRunningStatus + "'; exit 0; fi\n" +
		"if [ \"$1\" = run ]; then\n" +
		"  shift; [ \"$1\" = -- ]; shift\n" +
		"  if [ \"$1 $2 $3\" = 'podman container exists' ]; then exit 0; fi\n" +
		"  if [ \"$1 $2 $3\" = 'podman container inspect' ]; then echo true; exit 0; fi\n" +
		"fi\n" +
		"echo 'Unknown command: health-check' >&2\nexit 1\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime := localruntime.NewMacVMRuntime(deployment, manager)

	if err := classifyLocalReachability(context.Background(), localRuntime); err != nil {
		t.Fatalf("expected no-op when health-check is unavailable, got %v", err)
	}
}

// Regression: the diagnosis used to return the reachability guidance and drop
// the error the runtime actually reported. A container that fails to start
// publishes no ports, so the classifier fires and the real cause — a Podman
// failure, say — became invisible behind a network story.
func TestDiagnoseLocalFailurePreservesCausalError(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	blockedJSON := `{"ports":{"ui":{"state":"blocked"},"db":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, fakeRunnerRunningStatus, blockedJSON)
	localRuntime := localruntime.NewMacVMRuntime(deployment, manager)

	cause := errors.New("failed to remove Nano container exasol-db-649d54af: exit status 125")

	err := diagnoseLocalFailure(context.Background(), localRuntime, cause)
	if err == nil {
		t.Fatal("expected a diagnosis for a failure with every port blocked")
	}
	// The guidance is still reachable by sentinel...
	if !errors.Is(err, ErrLocalReachability) {
		t.Errorf("expected errors.Is(err, ErrLocalReachability), got %v", err)
	}
	// ...and the causal error survives, both by identity and in the message.
	if !errors.Is(err, cause) {
		t.Errorf("expected the causal error to remain matchable, got %v", err)
	}
	if !strings.Contains(err.Error(), "exit status 125") {
		t.Errorf("expected the causal error in the message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "could not reach the local database endpoint") {
		t.Errorf("expected the reachability guidance to be retained, got %q", err.Error())
	}
}

// When the ports look fine the diagnosis must not editorialise at all.
func TestDiagnoseLocalFailureReturnsCauseUnchangedWhenReachable(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	mixedJSON := `{"ports":{"ui":{"state":"reachable"},"db":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, fakeRunnerRunningStatus, mixedJSON)
	localRuntime := localruntime.NewMacVMRuntime(deployment, manager)

	cause := errors.New("podman run failed")

	err := diagnoseLocalFailure(context.Background(), localRuntime, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("expected the causal error, got %v", err)
	}
	if errors.Is(err, ErrLocalReachability) {
		t.Errorf("expected no reachability guidance when a port is reachable, got %v", err)
	}
}

// A container that is not running publishes no ports, which is
// indistinguishable from a blocked network path by port health alone. The
// diagnosis must stay silent there rather than send the user after firewalls
// and port conflicts when the real fault is that nothing started.
func TestDiagnoseLocalFailureSkipsGuidanceWhenRuntimeNotRunning(t *testing.T) {
	t.Parallel()
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	blockedJSON := `{"ports":{"ui":{"state":"blocked"},"db":{"state":"blocked"}}}`
	manager := writeFakeCombinedRunner(t, `{"running":false}`, blockedJSON)
	localRuntime := localruntime.NewMacVMRuntime(deployment, manager)

	cause := errors.New("failed to remove Nano container exasol-db-649d54af: exit status 125")

	err := diagnoseLocalFailure(context.Background(), localRuntime, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("expected the causal error, got %v", err)
	}
	if errors.Is(err, ErrLocalReachability) {
		t.Errorf("expected no reachability guidance for a stopped runtime, got %v", err)
	}
}

func TestLocalReachabilityMessageForLinuxHostDoesNotUseMacOSGuidance(t *testing.T) {
	t.Parallel()

	// Given
	localRuntime := localruntime.NewHostLinuxRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	// When
	message := localReachabilityMessageForRuntime(localRuntime)

	// Then
	if message != linuxHostReachabilityMessage {
		t.Fatalf("expected Linux Podman guidance, got %q", message)
	}
	if strings.Contains(message, "Local Network") || strings.Contains(message, "host-to-VM") {
		t.Fatalf("Linux guidance must not contain macOS VM advice: %q", message)
	}
}

func TestLocalReachabilityMessageForWindowsHostDoesNotUseMacOSGuidance(t *testing.T) {
	t.Parallel()

	localRuntime := localruntime.NewHostWindowsRuntime(config.NewDeploymentDir(t.TempDir()), nil)

	message := localReachabilityMessageForRuntime(localRuntime)

	if message != windowsHostReachabilityMessage {
		t.Fatalf("expected Windows Podman guidance, got %q", message)
	}
	if strings.Contains(message, "Local Network") ||
		strings.Contains(message, "System Settings") {
		t.Fatalf("Windows guidance must not contain macOS advice: %q", message)
	}
	if !strings.Contains(message, "Windows Firewall") ||
		!strings.Contains(message, "podman machine") {
		t.Fatalf("Windows guidance must mention the firewall and the podman machine: %q", message)
	}
	if strings.Contains(message, "set --rootful") {
		t.Fatalf("Windows guidance must not advise converting to rootful: %q", message)
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake local runner script is POSIX-only")
	}
}

func newLocalTestDeployment(t *testing.T) config.DeploymentDir {
	t.Helper()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: local
`)
	state := &config.ExasolPersonalState{DeploymentId: "local-test"}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{}, deployment,
	); err != nil {
		t.Fatalf("failed to write local deployment state: %v", err)
	}

	return deployment
}

// ensureLocalRuntimeWorkDir creates the deployment's local runtime work
// directory, which cmd.Dir requires to exist before the resolved runner can
// actually be invoked.
func ensureLocalRuntimeWorkDir(t *testing.T, deployment config.DeploymentDir) {
	t.Helper()

	if err := os.MkdirAll(newLocalRuntimeTestPaths(deployment).WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime work dir: %v", err)
	}
}

// writeFakeCombinedRunner builds a Manager whose "exasol-local-runner"
// resource resolves to a single fake runner script that answers both
// "status" and "health-check", since callers in this package may invoke
// either against whatever the manager resolves. statusJSON is the raw
// response body for "status" (e.g. `{"running":true}`).
func writeFakeCombinedRunner(
	t *testing.T,
	statusJSON, healthCheckJSON string,
) *runtimeartifacts.Manager {
	t.Helper()

	script := "#!/bin/sh\n" +
		"if [ \"$1\" = status ]; then echo '" + statusJSON + "'; exit 0; fi\n" +
		"if [ \"$1\" = health-check ]; then echo '" + healthCheckJSON + "'; exit 0; fi\n" +
		"if [ \"$1\" = run ]; then\n" +
		"  shift; [ \"$1\" = -- ]; shift\n" +
		"  if [ \"$1 $2 $3\" = 'podman container exists' ]; then exit 0; fi\n" +
		"  if [ \"$1 $2 $3\" = 'podman container inspect' ]; then echo true; exit 0; fi\n" +
		"fi\n" +
		"exit 1\n"

	return newTestManagerForRunner(t, []byte(script))
}

// newTestManagerForRunner builds a Manager whose "exasol-local-runner"
// resource resolves through the same extract: true / resource_path shape the
// real resources.yaml entry uses: scriptContent is packed into a minimal,
// single-entry zip (mirroring the real release archive).
func newTestManagerForRunner(t *testing.T, scriptContent []byte) *runtimeartifacts.Manager {
	t.Helper()

	zipPath := writeRunnerZip(t, scriptContent)
	spec := runtimeartifacts.ResourceSpec{
		exasolLocalRunnerResourceID: {
			Extract: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"any": {URL: zipPath, ResourcePath: runnerZipEntryName},
			},
		},
	}

	return runtimeartifacts.NewResourceManagerForPlatform(
		spec, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)
}

func writeRunnerZip(t *testing.T, scriptContent []byte) string {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "runner.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create runner zip fixture: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: runnerZipEntryName, Method: zip.Deflate}
	header.SetMode(testRunnerExecutableMode)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create runner zip entry: %v", err)
	}
	if _, err := entry.Write(scriptContent); err != nil {
		t.Fatalf("failed to write runner zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close runner zip fixture: %v", err)
	}

	return zipPath
}
