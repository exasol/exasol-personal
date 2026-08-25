// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package podmanmachine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installFakePodmanMachineShim drops a POSIX shell shim named "podman"
// into a fresh temp directory and sets PATH to that directory only.
// The shim dispatches on its argv using the provided body; the caller is
// responsible for handling any subcommand it expects to receive.
// Returns the argv log path.
func installFakePodmanMachineShim(t *testing.T, dispatch string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake podman shim uses a POSIX shell script; skipping on windows")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "podman.argv.log")
	script := "#!/bin/sh\n" +
		`printf 'podman' >> "` + logPath + `"` + "\n" +
		`for arg in "$@"; do printf ' %s' "$arg" >> "` + logPath + `"; done` + "\n" +
		`printf '\n' >> "` + logPath + `"` + "\n" +
		dispatch + "\n"
	//nolint:gosec // Test fixture must be executable.
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake podman shim: %v", err)
	}
	t.Setenv("PATH", dir)

	return logPath
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		t.Fatalf("read log: %v", err)
	}

	return string(data)
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestMachineExists_ReturnsTrueWhenNameFound(t *testing.T) {
	installFakePodmanMachineShim(t, `printf '`+DefaultMachineName+`\n'; exit 0`)

	exists, err := MachineExists(context.Background())
	if err != nil {
		t.Fatalf("MachineExists error: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestMachineExists_ReturnsFalseWhenListEmpty(t *testing.T) {
	installFakePodmanMachineShim(t, `exit 0`)

	exists, err := MachineExists(context.Background())
	if err != nil {
		t.Fatalf("MachineExists error: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false when podman lists no machines")
	}
}

// Regression: on Windows, podman appends "*" to the currently-active
// machine name even under --format "{{.Name}}". MachineExists must
// treat "podman-machine-default*" as the default machine.
//
//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestMachineExists_TolerantOfActiveMachineAsterisk(t *testing.T) {
	installFakePodmanMachineShim(t,
		`printf '`+DefaultMachineName+`*\n'; exit 0`)

	exists, err := MachineExists(context.Background())
	if err != nil {
		t.Fatalf("MachineExists error: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true when name has active-machine asterisk suffix")
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestMachineExists_PropagatesPodmanFailure(t *testing.T) {
	installFakePodmanMachineShim(t, `echo "provider missing" >&2; exit 125`)

	_, err := MachineExists(context.Background())
	if err == nil {
		t.Fatal("expected error when podman machine list fails")
	}
	if !strings.Contains(err.Error(), "podman machine list failed") {
		t.Errorf("expected wrapped error, got %v", err)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestMachineState_ReturnsTrimmedValue(t *testing.T) {
	installFakePodmanMachineShim(t, `printf '  Running  \n'; exit 0`)

	state, err := MachineState(context.Background())
	if err != nil {
		t.Fatalf("MachineState error: %v", err)
	}
	if state != "Running" {
		t.Errorf("got %q want %q", state, "Running")
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestInitMachine_UsesExpectedArgv(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, `exit 0`)

	err := InitMachine(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, 40)
	if err != nil {
		t.Fatalf("InitMachine error: %v", err)
	}
	got := readLog(t, logPath)
	want := "podman machine init --disk-size 40"
	if !strings.Contains(got, want) {
		t.Errorf("argv:\n  got:  %q\n  want substring: %q", got, want)
	}
	// The launcher never chooses a privilege mode; podman's default stands.
	if strings.Contains(got, "--rootful") {
		t.Errorf("init must not force a privilege mode, got %q", got)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestInitMachine_RejectsNonPositiveDiskSize(t *testing.T) {
	installFakePodmanMachineShim(t, `exit 0`)

	err := InitMachine(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, 0)
	if err == nil {
		t.Fatal("expected error on zero disk size")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("expected 'positive' in error, got %v", err)
	}
}

// dispatchScript builds a shell dispatch that handles the subcommands
// EnsureMachineRunning invokes. The behavior for each subcommand is
// injected via named parameters so tests can shape the machine's state.
//
// `machine stop` and `machine set` are deliberately dispatched to exit 64
// rather than 0: the launcher must never mutate the user's machine, so a
// call to either should fail the test loudly instead of passing silently.
type dispatchScript struct {
	// listOutput is what "podman machine list --format {{.Name}}" prints.
	listOutput string
	// stateOutput is what "podman machine inspect --format {{.State}}" prints.
	stateOutput string
}

func (d dispatchScript) render() string {
	// $1 $2 identifies the subcommand family: `machine list`, `machine
	// inspect`, `machine init`, `machine start`.
	return `if [ "$1 $2" = "machine list" ]; then printf '` +
		d.listOutput + `\n'; exit 0; fi
if [ "$1 $2" = "machine inspect" ] && [ "$4" = "{{.State}}" ]; then printf '` +
		d.stateOutput + `\n'; exit 0; fi
if [ "$1 $2" = "machine init" ]; then exit 0; fi
if [ "$1 $2" = "machine start" ]; then exit 0; fi
echo "unexpected podman command: $*" >&2
exit 64
`
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestEnsureMachineRunning_CreatesMachineWhenMissing(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput: "", // no machines
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureMachineRunning(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "machine init --disk-size 40") {
		t.Errorf("expected machine init in log:\n%s", log)
	}
	if strings.Contains(log, "--rootful") {
		t.Errorf("fresh install must not force a privilege mode, log:\n%s", log)
	}
	if !strings.Contains(log, "machine start") {
		t.Errorf("expected machine start in log:\n%s", log)
	}
	if !strings.Contains(out.String(), "Creating podman machine") {
		t.Errorf("expected create messaging, got %q", out.String())
	}
}

// The launcher keeps whatever privilege mode the machine already has: the
// database port is published on 127.0.0.1 explicitly, so rootless works and
// there is no reason to mutate a shared host resource. dispatchScript makes
// `machine stop`/`machine set` fail, so a regression here surfaces as an
// error rather than a silent conversion.
//
//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestEnsureMachineRunning_KeepsExistingMachineUntouched(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:  DefaultMachineName,
		stateOutput: "running",
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureMachineRunning(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "machine init") || strings.Contains(log, "machine start") {
		t.Errorf("no lifecycle changes should occur, log:\n%s", log)
	}
	if strings.Contains(log, "{{.Rootful}}") {
		t.Errorf("privilege mode is irrelevant and must not be queried, log:\n%s", log)
	}
	if out.Len() != 0 {
		t.Errorf("expected no user messaging on no-op path, got %q", out.String())
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestEnsureMachineRunning_StartsStoppedMachine(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:  DefaultMachineName,
		stateOutput: "stopped",
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureMachineRunning(context.Background(), &out, &outErr); err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "machine start") {
		t.Errorf("stopped machine must be started, log:\n%s", log)
	}
	if strings.Contains(log, "machine init") {
		t.Errorf("did not expect machine init, log:\n%s", log)
	}
	if !strings.Contains(out.String(), "starting it") {
		t.Errorf("expected 'starting it' message, got %q", out.String())
	}
}

// Regression: the deploy pipeline passes nil writers into Runtime.Prepare,
// which then forwards them here. Previously we hit fmt.Fprintln(nil, ...)
// on the "machine does not exist" branch and panicked with a nil deref.
//
//nolint:paralleltest // The test replaces process-wide PATH with a fake podman shim.
func TestEnsureMachineRunning_TolerantOfNilWriters(t *testing.T) {
	installFakePodmanMachineShim(t, dispatchScript{}.render())

	if err := EnsureMachineRunning(context.Background(), nil, nil); err != nil {
		t.Fatalf("EnsureMachineRunning with nil writers unexpected error: %v", err)
	}
}
