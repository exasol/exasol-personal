// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package podmanmachine wraps the "podman machine ..." subcommand family
// used by the Windows local runtime to manage the WSL2 backing VM
// (podman-for-windows also refers to it as "podman machine"). It exposes
// low-level primitives (exists/state/init/start/stop/set-rootful) plus
// an EnsureRootfulRunning orchestrator that implements the launcher's
// decision tree.
//
// Rootful is required on Windows: rootless podman-for-windows routes
// traffic through pasta, which resets long-lived TLS connections during
// the Exasol wss:// endpoint handshake. Rootful uses the WSL2 kernel's
// nftables path instead and does not exhibit the bug.
package podmanmachine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/prompt"
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
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
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

// MachineIsRootful reports whether the default machine is configured as
// rootful. Only meaningful when MachineExists returned true.
func MachineIsRootful(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, binary, "machine", "inspect",
		"--format", "{{.Rootful}}", DefaultMachineName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf(
			"podman machine inspect --format .Rootful failed (%w): %s",
			err, strings.TrimSpace(stderr.String()),
		)
	}
	switch strings.ToLower(strings.TrimSpace(stdout.String())) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf(
			"unexpected podman machine .Rootful value: %q",
			strings.TrimSpace(stdout.String()),
		)
	}
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
func InitMachine(
	ctx context.Context, out, outErr io.Writer, rootful bool, diskSizeGB int,
) error {
	if diskSizeGB <= 0 {
		return fmt.Errorf("podman machine init: disk size must be positive, got %d", diskSizeGB)
	}
	args := []string{"machine", "init", "--disk-size", strconv.Itoa(diskSizeGB)}
	if rootful {
		args = append(args, "--rootful")
	}
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

// StopMachine stops the default machine. Required before
// SetMachineRootful because podman rejects the mode change on a running
// machine. Runs quietly.
func StopMachine(ctx context.Context, out, outErr io.Writer) error {
	return runPodmanQuietly(ctx, out, outErr,
		"podman machine stop", "machine", "stop")
}

// SetMachineRootful flips the default machine to rootful. The machine
// must be stopped first. Runs quietly.
func SetMachineRootful(ctx context.Context, out, outErr io.Writer) error {
	return runPodmanQuietly(ctx, out, outErr,
		"podman machine set --rootful", "machine", "set", "--rootful")
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

// MachineStatus reports the state EnsureMachineRunning left the default
// machine in. Callers use Rootful to decide whether they need to apply
// container-level workarounds (e.g. pasta network flags on Windows
// rootless).
type MachineStatus struct {
	Rootful bool
}

// EnsureMachineRunning ensures the default podman machine exists and is
// running. It does NOT force rootful; if the machine already exists
// (rootful or rootless) it is kept as-is. A missing machine is created
// as rootless — rootless is the safer default and works correctly for
// Exasol on Windows once container-level pasta network flags are
// applied (see WindowsHostRuntime.Start). Idempotent.
//
// Returns the observed rootful/rootless status. On Windows the caller
// applies container-level pasta flags when Rootful is false.
func EnsureMachineRunning(
	ctx context.Context, out, outErr io.Writer,
) (MachineStatus, error) {
	out, outErr = discardIfNil(out), discardIfNil(outErr)
	exists, err := MachineExists(ctx)
	if err != nil {
		return MachineStatus{}, err
	}
	if !exists {
		fmt.Fprintln(out,
			"Creating podman machine (this may take a few minutes)...")
		if err := InitMachine(ctx, out, outErr, false, DefaultDiskSizeGB); err != nil {
			return MachineStatus{}, err
		}
		fmt.Fprintln(out, "Starting podman machine...")
		if err := StartMachine(ctx, out, outErr); err != nil {
			return MachineStatus{}, err
		}
		fmt.Fprintln(out, "Podman machine is ready.")

		return MachineStatus{Rootful: false}, nil
	}
	rootful, err := MachineIsRootful(ctx)
	if err != nil {
		return MachineStatus{}, err
	}
	state, err := MachineState(ctx)
	if err != nil {
		return MachineStatus{}, err
	}
	if !strings.EqualFold(state, "running") {
		fmt.Fprintf(out, "Podman machine is %s; starting it...\n", state)
		if err := StartMachine(ctx, out, outErr); err != nil {
			return MachineStatus{}, err
		}
	}

	return MachineStatus{Rootful: rootful}, nil
}

// EnsureRootfulRunning ensures the default podman machine exists, is
// rootful, and is running. Idempotent: safe to call on every Prepare
// and Start.
//
// If in refers to a terminal, the rootless→rootful conversion prompts
// for consent (default yes). Otherwise the conversion proceeds
// automatically. A missing machine is always created without a prompt.
//
// Decision tree:
//
//  1. Machine does not exist   → init rootful + start.
//  2. Machine exists, rootful, running   → nothing to do.
//  3. Machine exists, rootful, not running   → start.
//  4. Machine exists, rootless   → (prompt if interactive) → stop, set --rootful, start.
func EnsureRootfulRunning(
	ctx context.Context, in io.Reader, out, outErr io.Writer,
) error {
	out, outErr = discardIfNil(out), discardIfNil(outErr)
	exists, err := MachineExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintln(out,
			"Creating rootful podman machine (this may take a few minutes)...")
		if err := InitMachine(ctx, out, outErr, true, DefaultDiskSizeGB); err != nil {
			return err
		}
		fmt.Fprintln(out, "Starting podman machine...")
		if err := StartMachine(ctx, out, outErr); err != nil {
			return err
		}
		fmt.Fprintln(out, "Podman machine is ready.")

		return nil
	}
	rootful, err := MachineIsRootful(ctx)
	if err != nil {
		return err
	}
	if !rootful {
		fmt.Fprintln(out,
			"The default podman machine is rootless.")
		fmt.Fprintln(out,
			"Rootful is required because rootless podman-for-windows publishes container")
		fmt.Fprintln(out,
			"ports through gvproxy \u2192 pasta, and that path aborts every TLS handshake")
		fmt.Fprintln(out,
			"from Windows loopback (verified empirically with schannel curl; the peer")
		fmt.Fprintln(out,
			"closes the connection between ClientHello and ServerHello). Exasol's wss://")
		fmt.Fprintln(out,
			"endpoint therefore never opens on a rootless machine.")
		fmt.Fprintln(out,
			"Converting will stop the machine, apply the change, and start it again.")
		consent, err := prompt.YesNo(in, out,
			"Convert podman-machine-default to rootful now?", true)
		if err != nil {
			return fmt.Errorf("could not read rootful-conversion prompt: %w", err)
		}
		if !consent {
			return errors.New(
				"the launcher requires a rootful podman machine; " +
					"convert it manually with " +
					"`podman machine stop && podman machine set --rootful && podman machine start` " +
					"and re-run this command")
		}
		if err := StopMachine(ctx, out, outErr); err != nil {
			return err
		}
		if err := SetMachineRootful(ctx, out, outErr); err != nil {
			return err
		}
		if err := StartMachine(ctx, out, outErr); err != nil {
			return err
		}
		fmt.Fprintln(out, "Podman machine is now rootful.")

		return nil
	}
	state, err := MachineState(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(state, "running") {
		fmt.Fprintf(out, "Podman machine is rootful but %s; starting it...\n", state)
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
