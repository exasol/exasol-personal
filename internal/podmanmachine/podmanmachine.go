// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package podmanmachine wraps the "podman machine ..." subcommand family
// used by the Windows local runtime to manage the WSL2 backing VM
// (podman-for-windows also refers to it as "podman machine"). It exposes
// low-level primitives (exists/state/init/start) plus an
// EnsureMachineRunning orchestrator.
//
// The machine is used in whatever mode podman defaults to; the launcher
// never converts between rootless and rootful. Rootless works because
// PodmanInstall publishes the database port on 127.0.0.1 explicitly,
// which keeps WSL's localhost relay off the broken IPv6 path.
package podmanmachine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// DefaultMachineName is the machine name podman-for-windows uses when
// no name is specified on init. All operations in this package target
// this machine; multi-machine setups are out of scope.
const DefaultMachineName = "podman-machine-default"

// DefaultDiskSizeGB is the WSL2 backing-disk size passed to
// "podman machine init --disk-size". Sized to hold the Exasol nano
// image plus a working data directory with room to spare; the WSL2
// default is smaller and has been observed to run out during long
// integration runs.
const DefaultDiskSizeGB = 40

// binary is the podman executable name. Overridable so tests can stage
// a shim under a stable name on an isolated PATH.
var binary = "podman"

// MachineExists reports whether the default podman machine exists.
// Uses "podman machine list --format {{.Name}}" and matches by name so
// that transient errors (socket unreachable, provider not installed)
// surface as an error rather than a spurious false.
func MachineExists(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, binary, "machine", "list", "--format", "{{.Name}}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf(
			"podman machine list failed (%w): %s",
			err, strings.TrimSpace(stderr.String()),
		)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		// Podman on Windows appends "*" to the currently-active machine
		// name even when the output is templated via --format "{{.Name}}",
		// so strip a trailing asterisk before comparing.
		name := strings.TrimSuffix(strings.TrimSpace(line), "*")
		if name == DefaultMachineName {
			return true, nil
		}
	}

	return false, nil
}

// MachineState returns the machine's lifecycle state (e.g. "running",
// "stopped", "starting"). Only meaningful when MachineExists returned
// true. Callers usually compare against "running" case-insensitively.
func MachineState(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, binary, "machine", "inspect",
		"--format", "{{.State}}", DefaultMachineName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(
			"podman machine inspect --format .State failed (%w): %s",
			err, strings.TrimSpace(stderr.String()),
		)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// InitMachine initializes a new default machine with the given disk
// size. Streams output because the first-time WSL2 image download is a
// multi-minute step.
//
// The provider (WSL vs Hyper-V) is intentionally NOT forced via a flag:
// podman 5.x rejects --provider with "unknown flag", WSL is the default
// on every current release, and users who want Hyper-V can set
// CONTAINERS_MACHINE_PROVIDER=hyperv in their environment.
func InitMachine(ctx context.Context, out, outErr io.Writer, diskSizeGB int) error {
	if diskSizeGB <= 0 {
		return fmt.Errorf("podman machine init: disk size must be positive, got %d", diskSizeGB)
	}
	args := []string{"machine", "init", "--disk-size", strconv.Itoa(diskSizeGB)}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = out
	cmd.Stderr = outErr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman machine init failed: %w", err)
	}

	return nil
}

// StartMachine starts the default machine. Blocks until podman reports
// the machine is running or exits with an error. Runs quietly: podman's
// own stdout/stderr are only shown to the user if the command fails.
func StartMachine(ctx context.Context, out, outErr io.Writer) error {
	return runPodmanQuietly(ctx, out, outErr,
		"podman machine start", "machine", "start")
}

// runPodmanQuietly invokes a podman subcommand and captures its
// stdout/stderr into memory. On success, the captured output is
// discarded (podman-machine commands are chatty about their state on
// every start/stop even when nothing interesting happened). On
// failure, the captured output is replayed to outErr so the user has
// context for the error. InitMachine intentionally does NOT use this
// helper because its output is a genuine multi-minute WSL2 image
// download progress that users need to see.
func runPodmanQuietly(
	ctx context.Context, out, outErr io.Writer, description string, args ...string,
) error {
	out, outErr = discardIfNil(out), discardIfNil(outErr)
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stdout.Len() > 0 {
			_, _ = out.Write(stdout.Bytes())
		}
		if stderr.Len() > 0 {
			_, _ = outErr.Write(stderr.Bytes())
		}

		return fmt.Errorf("%s failed: %w", description, err)
	}

	return nil
}

// EnsureMachineRunning ensures the default podman machine exists and is
// running. A missing machine is created with podman's default rootless/
// rootful mode; an existing machine keeps whatever mode it has. Idempotent,
// so it is safe to call on both Prepare and Start.
func EnsureMachineRunning(ctx context.Context, out, outErr io.Writer) error {
	out, outErr = discardIfNil(out), discardIfNil(outErr)
	exists, err := MachineExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		_, _ = fmt.Fprintln(out,
			"Creating podman machine (this may take a few minutes)...")
		if err := InitMachine(ctx, out, outErr, DefaultDiskSizeGB); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "Starting podman machine...")
		if err := StartMachine(ctx, out, outErr); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "Podman machine is ready.")

		return nil
	}
	state, err := MachineState(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(state, "running") {
		_, _ = fmt.Fprintf(out, "Podman machine is %s; starting it...\n", state)
		if err := StartMachine(ctx, out, outErr); err != nil {
			return err
		}
	}

	return nil
}

// discardIfNil replaces a nil io.Writer with io.Discard so subsequent
// fmt.Fprintln calls do not nil-deref. The deploy pipeline passes nil
// writers into Runtime.Prepare by convention.
func discardIfNil(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}

	return w
}
