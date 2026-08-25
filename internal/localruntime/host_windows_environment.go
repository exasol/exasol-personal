// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/podmanmachine"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/winget"
)

// windowsHostEnvironmentPreparer satisfies the Windows prerequisites for the
// shared HostRuntime: a Podman on PATH (installing it via winget with
// approval when absent) and a running Podman machine to host the containers.
//
// Only the install is approval-gated. Creating or starting the machine is
// how Podman is normally used on Windows rather than a change to the user's
// system, and the launcher never alters an existing machine's configuration.
type windowsHostEnvironmentPreparer struct{}

// NewHostWindowsRuntime creates a Windows host runtime. The manager may be
// nil for operations that never invoke Podman, such as destroying a
// deployment that was never prepared.
func NewHostWindowsRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *HostRuntime {
	return newHostRuntime(deployment, manager, windowsHostEnvironmentPreparer{})
}

func (windowsHostEnvironmentPreparer) Platform() HostPlatform { return HostPlatformWindows }

func (windowsHostEnvironmentPreparer) EnsureReady(
	ctx context.Context,
	options PrepareOptions,
) error {
	if err := ensureWindowsPodmanAvailable(ctx, options); err != nil {
		return err
	}

	return podmanmachine.EnsureMachineRunning(ctx, options.Progress, options.Progress)
}

// EnsureStartable re-checks the Podman machine, which can be stopped between
// Prepare and Start by a reboot, a WSL shutdown, or the user. It needs no
// approval: starting a machine the launcher was already told to use is not a
// change to the host's configuration.
func (windowsHostEnvironmentPreparer) EnsureStartable(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	return podmanmachine.EnsureMachineRunning(ctx, out, outErr)
}

func (windowsHostEnvironmentPreparer) NewExecutionEnvironment(
	runtimeExec []string,
) localinstall.ExecutionEnvironment {
	return &windowsHostExecutionEnvironment{
		DirectExecutionEnvironment: localinstall.NewDirectExecutionEnvironment(runtimeExec),
	}
}

// ensureWindowsPodmanAvailable makes `podman` runnable from this process,
// installing it through winget if needed. Installation is gated on approval
// because it mutates state shared well beyond this deployment and can raise
// a UAC prompt.
func ensureWindowsPodmanAvailable(
	ctx context.Context,
	options PrepareOptions,
) error {
	if podmanOnPath() {
		return nil
	}
	// Stale-PATH recovery: Podman may already be installed by an earlier
	// winget run whose PATH updates this process never saw. Refreshing costs
	// one PowerShell call and can avoid a pointless reinstall prompt.
	if err := winget.EnsurePodmanOnPath(ctx); err == nil && podmanOnPath() {
		return nil
	}
	if err := winget.LookupWinget(); err != nil {
		return fmt.Errorf(
			"podman is required for local deployments but is not installed, and Windows "+
				"Package Manager (winget) is unavailable to install it; install "+
				"App Installer from the Microsoft Store, or install podman manually "+
				"from https://podman.io/, then re-run this command: %w", err)
	}

	request := HostChangeRequest{
		Kind: HostChangeInstallContainerRuntime,
		Commands: []HostCommand{
			{Name: "winget", Args: winget.PodmanInstallArgs()},
		},
	}
	if err := requireHostChangeApproval(ctx, options, request); err != nil {
		return err
	}

	writePreparationProgress(options.Progress, "Installing the local container runtime...")
	if err := winget.InstallPodman(ctx, options.Progress, options.Progress); err != nil {
		return fmt.Errorf(
			"failed to install podman through Windows Package Manager; install it "+
				"manually from https://podman.io/, then re-run this command: %w", err)
	}
	if err := winget.EnsurePodmanOnPath(ctx); err != nil {
		return fmt.Errorf("failed to refresh PATH after installing podman: %w", err)
	}
	if !podmanOnPath() {
		return errors.New(
			"the podman installation completed but podman is still not on PATH; " +
				"open a new terminal and re-run this command",
		)
	}

	return nil
}

func podmanOnPath() bool {
	_, err := exec.LookPath("podman")

	return err == nil
}

// requireHostChangeApproval denies the change when no approver was supplied,
// so a caller that forgets to wire one cannot silently mutate the host.
func requireHostChangeApproval(
	ctx context.Context,
	options PrepareOptions,
	request HostChangeRequest,
) error {
	if options.ApproveHostChange == nil {
		return fmt.Errorf("host change %q requires explicit approval", request.Kind)
	}
	approved, err := options.ApproveHostChange(ctx, request)
	if err != nil {
		return err
	}
	if !approved {
		return fmt.Errorf("host change %q was not approved", request.Kind)
	}

	return nil
}

func writePreparationProgress(writer io.Writer, message string) {
	if writer != nil {
		_, _ = fmt.Fprintln(writer, message)
	}
}

// windowsHostExecutionEnvironment embeds DirectExecutionEnvironment for
// every command except Sync, which is redirected into the Podman machine via
// `podman machine ssh -- sync`.
//
// The data directory is a Windows path bind-mounted into the machine, so the
// bytes end up on the Windows filesystem. The writes are still buffered by
// the machine's kernel on the way there, and Windows offers no host-side
// `sync` the launcher could invoke, so the flush has to happen inside the
// machine. This does not guarantee the Windows filesystem has committed
// them; it only removes the machine's buffering from the path.
type windowsHostExecutionEnvironment struct {
	*localinstall.DirectExecutionEnvironment
}

func (*windowsHostExecutionEnvironment) Sync(
	ctx context.Context, out, outErr io.Writer,
) error {
	// podman machine ssh produces no output on a successful sync; we
	// buffer both streams and only replay them on failure so a healthy
	// sync stays invisible.
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "podman", "machine", "ssh", "--", "sync")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if out != nil && stdout.Len() > 0 {
			_, _ = out.Write(stdout.Bytes())
		}
		if outErr != nil && stderr.Len() > 0 {
			_, _ = outErr.Write(stderr.Bytes())
		}

		return fmt.Errorf("podman machine ssh -- sync failed: %w", err)
	}

	return nil
}
