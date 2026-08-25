// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	windowsGOOS        = "windows"
	runnerZipEntryName = "launcher"
)

func newTestManagerForRunner(t *testing.T, scriptContent []byte) *runtimeartifacts.Manager {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "runner.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create runner zip: %v", err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: runnerZipEntryName, Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create runner entry: %v", err)
	}
	if _, err := entry.Write(scriptContent); err != nil {
		t.Fatalf("failed to write runner entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close runner zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close runner fixture: %v", err)
	}

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

func writeExecutableTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, executableFileMode); err != nil {
		t.Fatalf("failed to write executable fixture: %v", err)
	}
}

//nolint:paralleltest // test runner scripts share process environment and fork executable files.
func TestMacVMRuntimeLifecycleUsesV2RunnerThenSharedInstall(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	runnerScript := fakeV2RunnerScript(eventsPath, 28563)
	localRuntime := NewMacVMRuntime(deployment, newTestManagerForRunner(t, []byte(runnerScript)))
	install := &recordingLocalInstall{eventsPath: eventsPath, running: true}
	localRuntime.installFactory = func(string) (localinstall.LocalInstall, error) {
		return install, nil
	}

	if err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		RuntimeConfig: RuntimeConfig{Ports: "db:28563"},
		CPUCount:      4,
		MemoryMB:      8192,
		DataSizeGB:    100,
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if install.startConfig.ContainerDBPort != nanoDBPort ||
		install.startConfig.DataDir != vmNanoDataDir {
		t.Fatalf("unexpected guest Podman config: %#v", install.startConfig)
	}
	if len(install.startConfig.LegacyContainerNames) != 1 ||
		install.startConfig.LegacyContainerNames[0] != legacyNanoContainer {
		t.Fatalf("expected legacy container migration config, got %#v", install.startConfig)
	}
	endpoint, err := localRuntime.ReadEndpoints()
	if err != nil || endpoint.DBPort != 28563 {
		t.Fatalf("unexpected endpoint %#v, err=%v", endpoint, err)
	}
	status, err := localRuntime.Status(context.Background())
	if err != nil || !status.Running {
		t.Fatalf("expected combined running status, got %#v err=%v", status, err)
	}
	if err := localRuntime.Stop(context.Background(), nil, nil); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	events, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read lifecycle events: %v", err)
	}
	want := []string{"runner-init", "runner-start", "install-start", "install-stop", "runner-stop"}
	for _, event := range want {
		if !strings.Contains(string(events), event+"\n") {
			t.Fatalf("missing %q in lifecycle events:\n%s", event, events)
		}
	}
	if strings.Index(string(events), "install-stop") >
		strings.Index(string(events), "runner-stop") {
		t.Fatalf("expected container cleanup before VM stop:\n%s", events)
	}

	args, err := os.ReadFile(filepath.Join(localRuntime.paths.WorkDir, "start-args"))
	if err != nil {
		t.Fatalf("failed to read start args: %v", err)
	}
	if string(args) != "--forward db:8563:28563 4 8192 100" {
		t.Fatalf("unexpected v2 runner start args %q", args)
	}
}

//nolint:paralleltest // test runner scripts fork executable files.
func TestMacVMRuntimeStartStopsVMWhenInstallFails(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	eventsPath := filepath.Join(t.TempDir(), "events")
	localRuntime := NewMacVMRuntime(
		deployment,
		newTestManagerForRunner(t, []byte(fakeV2RunnerScript(eventsPath, 28563))),
	)
	localRuntime.installFactory = func(string) (localinstall.LocalInstall, error) {
		return &recordingLocalInstall{
			eventsPath: eventsPath,
			startErr:   errors.New("podman failed"),
		}, nil
	}
	if err := localRuntime.Prepare(context.Background(), nil, nil, PrepareOptions{}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	err := localRuntime.Start(context.Background(), nil, nil, VMConfig{
		CPUCount: 2, MemoryMB: 4096, DataSizeGB: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "podman failed") {
		t.Fatalf("expected install failure, got %v", err)
	}
	events, readErr := os.ReadFile(eventsPath)
	if readErr != nil {
		t.Fatalf("failed to read lifecycle events: %v", readErr)
	}
	if !strings.Contains(string(events), "install-start\nrunner-stop\n") {
		t.Fatalf("expected failed installation to stop VM, got:\n%s", events)
	}
}

//nolint:paralleltest // test runner scripts fork executable fixtures.
func TestMacVMRuntimeShellsDelegateToRunner(t *testing.T) {
	requirePOSIXRunnerTest(t)

	deployment := config.NewDeploymentDir(t.TempDir())
	state := &config.ExasolPersonalState{DeploymentId: "shell-test"}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateRunning{}, deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "runner-args")
	runnerScript := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
`, logPath)
	localRuntime := NewMacVMRuntime(
		deployment, newTestManagerForRunner(t, []byte(runnerScript)),
	)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, dirMode); err != nil {
		t.Fatalf("failed to create runtime work dir: %v", err)
	}

	if err := localRuntime.OpenHostShell(
		context.Background(), strings.NewReader(""), io.Discard, io.Discard,
	); err != nil {
		t.Fatalf("host shell failed: %v", err)
	}
	if err := localRuntime.OpenContainerShell(
		context.Background(), strings.NewReader(""), io.Discard, io.Discard,
	); err != nil {
		t.Fatalf("container shell failed: %v", err)
	}

	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read runner args: %v", err)
	}
	want := strings.Join([]string{
		"run",
		"run --tty -- sh -c " + containerShellScript + " sh exasol-db-shell-test",
		"",
	}, "\n")
	if string(args) != want {
		t.Fatalf("runner shell args = %q, want %q", args, want)
	}
}

func fakeV2RunnerScript(eventsPath string, hostDBPort int) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
events=%q
case "$1" in
  version)
    printf 'v2.0.0-dev\n'
    ;;
  init)
    mkdir -p vm vm-shared
    printf 'runner-init\n' >> "$events"
    ;;
  start)
    shift
    printf '%%s' "$*" > start-args
    touch running
    printf 'runner-start\n' >> "$events"
    cat > vm-state.json <<'EOF'
{
  "vm_name":"test-vm",
  "shared_dir":"./vm-shared",
  "forwards":{"db":{"guest_port":8563,"host_port":%d}}
}
EOF
    ;;
  status)
    if [ -f running ]; then printf '{"running":true}\n'; else printf '{"running":false}\n'; fi
    ;;
  stop)
    rm -f running vm-state.json
    printf 'runner-stop\n' >> "$events"
    ;;
  health-check)
    printf '{"ports":{"db":{"state":"reachable"}}}\n'
    ;;
  *)
    printf 'unexpected command: %%s\n' "$1" >&2
    exit 2
    ;;
esac
`, eventsPath, hostDBPort)
}

type recordingLocalInstall struct {
	eventsPath  string
	startConfig localinstall.StartConfig
	startErr    error
	running     bool
}

func (install *recordingLocalInstall) Start(
	_ context.Context,
	_, _ io.Writer,
	startConfig localinstall.StartConfig,
) error {
	install.startConfig = startConfig
	install.record("install-start")
	if install.startErr == nil {
		install.running = true
	}

	return install.startErr
}

func (install *recordingLocalInstall) Stop(context.Context, io.Writer, io.Writer) error {
	install.record("install-stop")
	install.running = false

	return nil
}

func (install *recordingLocalInstall) Status(
	context.Context,
	io.Writer,
	io.Writer,
) (*localinstall.InstallStatus, error) {
	return &localinstall.InstallStatus{Running: install.running}, nil
}

func (install *recordingLocalInstall) Destroy(ctx context.Context, out, outErr io.Writer) error {
	return install.Stop(ctx, out, outErr)
}

func (install *recordingLocalInstall) record(event string) {
	file, err := os.OpenFile(
		install.eventsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600,
	)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintln(file, event)
}
