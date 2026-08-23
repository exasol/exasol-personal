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

func TestMachineIsRootful_TrueAndFalseValues(t *testing.T) {
	for _, tc := range []struct {
		out  string
		want bool
	}{
		{"true", true},
		{"false", false},
	} {
		installFakePodmanMachineShim(t, `printf '`+tc.out+`\n'; exit 0`)

		got, err := MachineIsRootful(context.Background())
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.out, err)
		}
		if got != tc.want {
			t.Errorf("%q: got %v want %v", tc.out, got, tc.want)
		}
	}
}

func TestMachineIsRootful_RejectsUnexpectedOutput(t *testing.T) {
	installFakePodmanMachineShim(t, `printf 'maybe\n'; exit 0`)

	_, err := MachineIsRootful(context.Background())
	if err == nil {
		t.Fatal("expected error on unexpected .Rootful value")
	}
	if !strings.Contains(err.Error(), "unexpected") {
		t.Errorf("expected 'unexpected' in error, got %v", err)
	}
}

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

func TestInitMachine_RootfulPassesRootfulFlag(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, `exit 0`)

	err := InitMachine(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, true, 40)
	if err != nil {
		t.Fatalf("InitMachine error: %v", err)
	}
	got := readLog(t, logPath)
	want := "podman machine init --disk-size 40 --rootful"
	if !strings.Contains(got, want) {
		t.Errorf("argv did not include rootful flag:\n  got:  %q\n  want substring: %q", got, want)
	}
}

func TestInitMachine_RejectsNonPositiveDiskSize(t *testing.T) {
	installFakePodmanMachineShim(t, `exit 0`)

	err := InitMachine(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, true, 0)
	if err == nil {
		t.Fatal("expected error on zero disk size")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("expected 'positive' in error, got %v", err)
	}
}

func TestSetMachineRootful_UsesExpectedArgv(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, `exit 0`)

	if err := SetMachineRootful(
		context.Background(), &bytes.Buffer{}, &bytes.Buffer{},
	); err != nil {
		t.Fatalf("SetMachineRootful error: %v", err)
	}
	got := readLog(t, logPath)
	want := "podman machine set --rootful"
	if !strings.Contains(got, want) {
		t.Errorf("argv:\n  got:  %q\n  want substring: %q", got, want)
	}
}

// dispatchScript builds a shell dispatch that handles the subcommands
// EnsureRootfulRunning invokes. The behavior for each subcommand is
// injected via named parameters so tests can shape the machine's state.
type dispatchScript struct {
	// listOutput is what "podman machine list --format {{.Name}}" prints.
	listOutput string
	// rootfulOutput is what "podman machine inspect --format {{.Rootful}}" prints.
	rootfulOutput string
	// stateOutput is what "podman machine inspect --format {{.State}}" prints.
	stateOutput string
}

func (d dispatchScript) render() string {
	// $1 $2 identifies the subcommand family: `machine list`, `machine
	// inspect`, `machine init`, `machine start`, `machine stop`, `machine set`.
	// For `machine inspect`, $4 disambiguates {{.Rootful}} vs {{.State}}.
	return `if [ "$1 $2" = "machine list" ]; then printf '` +
		d.listOutput + `\n'; exit 0; fi
if [ "$1 $2" = "machine inspect" ] && [ "$4" = "{{.Rootful}}" ]; then printf '` +
		d.rootfulOutput + `\n'; exit 0; fi
if [ "$1 $2" = "machine inspect" ] && [ "$4" = "{{.State}}" ]; then printf '` +
		d.stateOutput + `\n'; exit 0; fi
if [ "$1 $2" = "machine init" ]; then exit 0; fi
if [ "$1 $2" = "machine start" ]; then exit 0; fi
if [ "$1 $2" = "machine stop" ]; then exit 0; fi
if [ "$1 $2" = "machine set" ]; then exit 0; fi
echo "unexpected podman command: $*" >&2
exit 64
`
}

func TestEnsureRootfulRunning_CreatesMachineWhenMissing(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput: "", // no machines
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureRootfulRunning(context.Background(), nil, &out, &outErr); err != nil {
		t.Fatalf("EnsureRootfulRunning error: %v", err)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "machine init --disk-size 40 --rootful") {
		t.Errorf("expected rootful init in log:\n%s", log)
	}
	if !strings.Contains(log, "machine start") {
		t.Errorf("expected machine start in log:\n%s", log)
	}
	if !strings.Contains(out.String(), "Creating rootful podman machine") {
		t.Errorf("expected create messaging, got %q", out.String())
	}
}

func TestEnsureRootfulRunning_NoOpWhenRootfulAndRunning(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:    DefaultMachineName,
		rootfulOutput: "true",
		stateOutput:   "running",
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureRootfulRunning(context.Background(), nil, &out, &outErr); err != nil {
		t.Fatalf("EnsureRootfulRunning error: %v", err)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "machine init") {
		t.Errorf("did not expect machine init, log:\n%s", log)
	}
	if strings.Contains(log, "machine start") {
		t.Errorf("did not expect machine start, log:\n%s", log)
	}
	if out.Len() != 0 {
		t.Errorf("expected no user messaging on no-op path, got %q", out.String())
	}
}

func TestEnsureRootfulRunning_StartsStoppedRootfulMachine(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:    DefaultMachineName,
		rootfulOutput: "true",
		stateOutput:   "stopped",
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureRootfulRunning(context.Background(), nil, &out, &outErr); err != nil {
		t.Fatalf("EnsureRootfulRunning error: %v", err)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "machine start") {
		t.Errorf("expected machine start, log:\n%s", log)
	}
	if strings.Contains(log, "machine init") {
		t.Errorf("did not expect machine init, log:\n%s", log)
	}
	if !strings.Contains(out.String(), "starting it") {
		t.Errorf("expected 'starting it' message, got %q", out.String())
	}
}

func TestEnsureRootfulRunning_ConvertsRootlessToRootful(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:    DefaultMachineName,
		rootfulOutput: "false",
	}.render())

	var out, outErr bytes.Buffer
	if err := EnsureRootfulRunning(context.Background(), nil, &out, &outErr); err != nil {
		t.Fatalf("EnsureRootfulRunning error: %v", err)
	}
	log := readLog(t, logPath)
	// Order matters: stop, set --rootful, start.
	stopIdx := strings.Index(log, "machine stop")
	setIdx := strings.Index(log, "machine set --rootful")
	startIdx := strings.Index(log, "machine start")
	if stopIdx < 0 || setIdx < 0 || startIdx < 0 {
		t.Fatalf("expected stop/set/start in log:\n%s", log)
	}
	if !(stopIdx < setIdx && setIdx < startIdx) {
		t.Errorf("expected stop→set→start ordering, got:\n%s", log)
	}
	if !strings.Contains(out.String(), "Podman machine is now rootful") {
		t.Errorf("expected conversion completion messaging, got %q", out.String())
	}
}

// Regression: the deploy pipeline passes nil writers into Runtime.Prepare,
// which then forwards them here. Previously we hit fmt.Fprintln(nil, ...)
// on the "machine does not exist" branch and panicked with a nil deref.
func TestEnsureRootfulRunning_TolerantOfNilWriters(t *testing.T) {
	installFakePodmanMachineShim(t, dispatchScript{}.render())

	if err := EnsureRootfulRunning(context.Background(), nil, nil, nil); err != nil {
		t.Fatalf("EnsureRootfulRunning with nil writers unexpected error: %v", err)
	}
}

func TestEnsureMachineRunning_CreatesRootlessWhenMissing(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput: "", // no machines
	}.render())

	var out, outErr bytes.Buffer
	status, err := EnsureMachineRunning(context.Background(), &out, &outErr)
	if err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	if status.Rootful {
		t.Errorf("fresh install must produce rootless machine, got %+v", status)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "machine init --disk-size 40") {
		t.Errorf("expected machine init in log:\n%s", log)
	}
	if strings.Contains(log, "--rootful") {
		t.Errorf("fresh install must NOT pass --rootful, log:\n%s", log)
	}
}

// The behavioural change from the "always convert rootless to rootful"
// policy: an existing rootless machine is now kept rootless. The Windows
// runtime layers container-level pasta workaround flags on top instead of
// mutating the user's machine.
func TestEnsureMachineRunning_KeepsExistingRootlessMachine(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:    DefaultMachineName,
		rootfulOutput: "false",
		stateOutput:   "running",
	}.render())

	var out, outErr bytes.Buffer
	status, err := EnsureMachineRunning(context.Background(), &out, &outErr)
	if err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	if status.Rootful {
		t.Errorf("existing rootless machine must be reported as rootless, got %+v", status)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "machine set --rootful") {
		t.Errorf("must not convert existing rootless machine, log:\n%s", log)
	}
	if strings.Contains(log, "machine stop") {
		t.Errorf("must not stop existing rootless machine, log:\n%s", log)
	}
}

func TestEnsureMachineRunning_StartsStoppedRootfulMachine(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:    DefaultMachineName,
		rootfulOutput: "true",
		stateOutput:   "stopped",
	}.render())

	var out, outErr bytes.Buffer
	status, err := EnsureMachineRunning(context.Background(), &out, &outErr)
	if err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	if !status.Rootful {
		t.Errorf("existing rootful machine must be reported as rootful, got %+v", status)
	}
	log := readLog(t, logPath)
	if !strings.Contains(log, "machine start") {
		t.Errorf("stopped machine must be started, log:\n%s", log)
	}
}

func TestEnsureMachineRunning_NoOpOnRunningRootfulMachine(t *testing.T) {
	logPath := installFakePodmanMachineShim(t, dispatchScript{
		listOutput:    DefaultMachineName,
		rootfulOutput: "true",
		stateOutput:   "running",
	}.render())

	var out, outErr bytes.Buffer
	status, err := EnsureMachineRunning(context.Background(), &out, &outErr)
	if err != nil {
		t.Fatalf("EnsureMachineRunning error: %v", err)
	}
	if !status.Rootful {
		t.Errorf("running rootful machine must be reported as rootful, got %+v", status)
	}
	log := readLog(t, logPath)
	if strings.Contains(log, "machine start") || strings.Contains(log, "machine init") {
		t.Errorf("no lifecycle changes should occur, log:\n%s", log)
	}
}
