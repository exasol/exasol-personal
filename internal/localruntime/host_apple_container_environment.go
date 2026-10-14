// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/exasol/exasol-personal/internal/applecontainer"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"github.com/exasol/exasol-personal/internal/util"
)

// appleContainerMinimumMacOSVersion is the lowest macOS major version Apple
// documents `container` as running on at all. macOS 15 has known vmnet
// limitations (single network, a subnet-assignment race on the first
// container start); macOS 26 is recommended, but 15 is not rejected outright.
const appleContainerMinimumMacOSVersion = 15

// appleContainerHostEnvironmentPreparer satisfies the prerequisites for the
// shared HostRuntime using Apple's `container` CLI instead of Podman: the
// CLI itself (installing it via Homebrew, or its signed installer package
// when Homebrew is unavailable, with approval either way) and a running
// `container` system service to host the containers.
//
// Only installing the CLI is approval-gated. Starting its already-installed
// system service is how `container` is normally used rather than a change to
// the user's system.
type appleContainerHostEnvironmentPreparer struct{}

// NewHostAppleContainerRuntime creates a runtime for the local-apple-container
// preset. The manager may be nil for operations that never invoke the CLI,
// such as destroying a deployment that was never prepared.
func NewHostAppleContainerRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *HostRuntime {
	return newHostRuntimeWithInstall(
		deployment,
		manager,
		appleContainerHostEnvironmentPreparer{},
		newAppleContainerInstall,
	)
}

func newAppleContainerInstall(
	deployment config.DeploymentDir,
	environment localinstall.ExecutionEnvironment,
	resolveImage func(context.Context) (localinstall.RuntimePath, error),
	slcStagingDir, slcStatusPath localinstall.RuntimePath,
) localinstall.LocalInstall {
	return localinstall.NewAppleContainerInstallWithEnvironment(
		deployment, environment, resolveImage, slcStagingDir, slcStatusPath,
	)
}

func (appleContainerHostEnvironmentPreparer) Platform() HostPlatform {
	return HostPlatformAppleContainer
}

func (appleContainerHostEnvironmentPreparer) EnsureReady(
	ctx context.Context,
	options PrepareOptions,
) error {
	// The macOS-version floor needs a real syscall, so it is checked here
	// rather than at runtime-selection time (internal/deploy.
	// validateAppleContainerPlatform), which only does the syscall-free
	// goos/goarch check so runtime selection stays testable on any platform.
	if err := ensureAppleContainerMacOSVersion(ctx); err != nil {
		return err
	}
	if err := ensureAppleContainerCLIAvailable(ctx, options); err != nil {
		return err
	}

	return ensureAppleContainerSystemRunning(ctx, options.Progress, options.Progress)
}

func ensureAppleContainerMacOSVersion(ctx context.Context) error {
	majorVersion, err := util.GetMacOSMajorVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to determine the macOS version: %w", err)
	}
	if majorVersion < appleContainerMinimumMacOSVersion {
		return fmt.Errorf(
			"the local-apple-container preset requires macOS %d or later (detected macOS %d)",
			appleContainerMinimumMacOSVersion, majorVersion,
		)
	}

	return nil
}

// EnsureStartable re-checks the `container` system service, which can be
// stopped between Prepare and Start by a reboot or the user. It needs no
// approval: starting a service the launcher already confirmed is installed
// is not a change to the host's configuration.
func (appleContainerHostEnvironmentPreparer) EnsureStartable(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	if err := applecontainer.LookupCLI(); err != nil {
		return err
	}

	return ensureAppleContainerSystemRunning(ctx, out, outErr)
}

// ContainerHostRunning reads the `container` system service's state without
// starting it. The service is host-wide state that a reboot or the user can
// stop at any time, and every container of every deployment stops with it.
func (appleContainerHostEnvironmentPreparer) ContainerHostRunning(
	ctx context.Context,
) (bool, error) {
	if err := applecontainer.LookupCLI(); err != nil {
		return false, nil //nolint:nilerr // CLI absent means nothing hosts containers, not an error here.
	}

	return exec.CommandContext(ctx, applecontainer.CLIBinary, "system", "status").Run() == nil, nil
}

func (appleContainerHostEnvironmentPreparer) NewExecutionEnvironment(
	runtimeExec []string,
) localinstall.ExecutionEnvironment {
	return localinstall.NewDirectExecutionEnvironment(runtimeExec)
}

// ensureAppleContainerCLIAvailable makes `container` runnable from this
// process, installing it through Homebrew when available or Apple's signed
// installer package otherwise. Installation is gated on approval because it
// mutates host state shared well beyond this deployment.
func ensureAppleContainerCLIAvailable(
	ctx context.Context,
	options PrepareOptions,
) error {
	if err := applecontainer.LookupCLI(); err == nil {
		return nil
	}

	if brewErr := applecontainer.LookupBrew(); brewErr == nil {
		return installAppleContainerViaBrew(ctx, options)
	}

	return installAppleContainerViaPackage(ctx, options)
}

func installAppleContainerViaBrew(ctx context.Context, options PrepareOptions) error {
	request := HostChangeRequest{
		Kind: HostChangeInstallAppleContainer,
		Commands: []HostCommand{
			{Name: "brew", Args: applecontainer.BrewInstallArgs()},
		},
	}
	if err := requireHostChangeApproval(ctx, options, request); err != nil {
		return err
	}

	writePreparationProgress(options.Progress, "Installing the local container runtime through Homebrew...")
	if err := applecontainer.InstallViaBrew(ctx, options.Progress, options.Progress); err != nil {
		return fmt.Errorf(
			"failed to install the `container` CLI through Homebrew; install it "+
				"manually from https://github.com/apple/container, then re-run this command: %w",
			err,
		)
	}

	return verifyAppleContainerOnPath()
}

func installAppleContainerViaPackage(ctx context.Context, options PrepareOptions) error {
	manager := runtimeartifacts.FromContext(ctx)
	packagePath, err := applecontainer.ResolveInstallerPackage(ctx, manager)
	if err != nil {
		return fmt.Errorf(
			"the `container` CLI is required for the local-apple-container preset but is not "+
				"installed, Homebrew is unavailable to install it, and its signed installer "+
				"package could not be resolved; install it manually from "+
				"https://github.com/apple/container, then re-run this command: %w",
			err,
		)
	}

	installerCommand := applecontainer.InstallerCommand(packagePath)
	request := HostChangeRequest{
		Kind: HostChangeInstallAppleContainer,
		Commands: []HostCommand{
			{Name: installerCommand[0], Args: installerCommand[1:]},
		},
	}
	if err := requireHostChangeApproval(ctx, options, request); err != nil {
		return err
	}

	writePreparationProgress(
		options.Progress, "Installing the local container runtime from Apple's signed installer...",
	)
	if err := applecontainer.InstallViaPackage(
		ctx, packagePath, options.Progress, options.Progress,
	); err != nil {
		return fmt.Errorf(
			"failed to install the `container` CLI from its signed installer package; install it "+
				"manually from https://github.com/apple/container, then re-run this command: %w",
			err,
		)
	}

	return verifyAppleContainerOnPath()
}

func verifyAppleContainerOnPath() error {
	if err := applecontainer.LookupCLI(); err != nil {
		return errors.New(
			"the `container` CLI installation completed but it is still not on PATH; " +
				"open a new terminal and re-run this command",
		)
	}

	return nil
}

// ensureAppleContainerSystemRunning starts the `container` system service if
// it is not already running. Starting it is not itself approval-gated:
// unlike installing the CLI, it changes nothing beyond this process's view
// of an already-installed, already-approved tool.
func ensureAppleContainerSystemRunning(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	if exec.CommandContext(ctx, applecontainer.CLIBinary, "system", "status").Run() == nil {
		return nil
	}

	writePreparationProgress(outErr, "Starting the local container runtime...")
	cmd := exec.CommandContext(ctx, applecontainer.CLIBinary, "system", "start")
	cmd.Stdout = out
	cmd.Stderr = outErr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start the local container runtime: %w", err)
	}

	return nil
}
