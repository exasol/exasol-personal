// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	HostChangeInstallContainerRuntime HostChangeKind = "install-container-runtime"
	HostChangeEnablePrivilegedRuntime HostChangeKind = "enable-privileged-runtime"

	windowsDefaultMachineName = "podman-machine-default"
	windowsMachineDiskSizeGB  = "40"
)

var (
	windowsPodmanInstallCommand = HostCommand{
		Name: "winget",
		Args: []string{
			"install", "--exact", "--id", "RedHat.Podman",
			"--source", "winget",
			"--accept-source-agreements",
			"--accept-package-agreements",
		},
	}
	windowsRegisteredPathScript = `$m = [Environment]::GetEnvironmentVariable("Path", "Machine")
$u = [Environment]::GetEnvironmentVariable("Path", "User")
if ($m -and $u) { "$m;$u" } elseif ($m) { $m } elseif ($u) { $u } else { "" }`
)

type windowsHostCommandRunner interface {
	Output(ctx context.Context, command HostCommand) (string, error)
	Run(ctx context.Context, progress io.Writer, command HostCommand) error
	Getenv(name string) string
	Setenv(name, value string) error
}

type osWindowsHostCommandRunner struct{}

func (osWindowsHostCommandRunner) Output(
	ctx context.Context,
	command HostCommand,
) (string, error) {
	output, err := exec.CommandContext(ctx, command.Name, command.Args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"%s failed: %w: %s",
			formatHostCommand(command), err, strings.TrimSpace(string(output)),
		)
	}

	return strings.TrimSpace(string(output)), nil
}

func (osWindowsHostCommandRunner) Run(
	ctx context.Context,
	progress io.Writer,
	command HostCommand,
) error {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", formatHostCommand(command), err)
	}

	return nil
}

func (osWindowsHostCommandRunner) Getenv(name string) string {
	return os.Getenv(name)
}

func (osWindowsHostCommandRunner) Setenv(name, value string) error {
	return os.Setenv(name, value)
}

type windowsHostEnvironmentPreparer struct {
	runner windowsHostCommandRunner
}

func NewHostWindowsRuntime(
	deployment config.DeploymentDir,
	manager *runtimeartifacts.Manager,
) *HostRuntime {
	return newHostRuntime(deployment, manager, windowsHostEnvironmentPreparer{
		runner: osWindowsHostCommandRunner{},
	})
}

func (preparer windowsHostEnvironmentPreparer) EnsureReady(
	ctx context.Context,
	options PrepareOptions,
) error {
	if err := preparer.ensurePodman(ctx, options); err != nil {
		return err
	}

	return preparer.ensureDefaultMachine(ctx, options)
}

func (preparer windowsHostEnvironmentPreparer) ensurePodman(
	ctx context.Context,
	options PrepareOptions,
) error {
	if preparer.podmanAvailable(ctx) {
		return nil
	}
	if err := preparer.refreshRegisteredPath(ctx); err != nil {
		return err
	}
	if preparer.podmanAvailable(ctx) {
		return nil
	}

	request := HostChangeRequest{
		Kind:     HostChangeInstallContainerRuntime,
		Commands: []HostCommand{windowsPodmanInstallCommand},
	}
	if err := requireHostChangeApproval(ctx, options, request); err != nil {
		return err
	}
	writePreparationProgress(options.Progress, "Installing the local container runtime...")
	if err := preparer.runner.Run(ctx, options.Progress, windowsPodmanInstallCommand); err != nil {
		return fmt.Errorf("failed to install Podman: %w", err)
	}
	if err := preparer.refreshRegisteredPath(ctx); err != nil {
		return err
	}
	if _, err := preparer.runner.Output(ctx, hostCommand("podman", "--version")); err != nil {
		return fmt.Errorf("podman is unavailable after installation: %w", err)
	}

	return nil
}

func (preparer windowsHostEnvironmentPreparer) podmanAvailable(ctx context.Context) bool {
	_, err := preparer.runner.Output(ctx, hostCommand("podman", "--version"))

	return err == nil
}

func (preparer windowsHostEnvironmentPreparer) refreshRegisteredPath(
	ctx context.Context,
) error {
	registeredPath, err := preparer.runner.Output(ctx, hostCommand(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", windowsRegisteredPathScript,
	))
	if err != nil {
		return fmt.Errorf("failed to refresh the registered Windows PATH: %w", err)
	}
	mergedPath := mergeWindowsPath(preparer.runner.Getenv("PATH"), registeredPath)
	if err := preparer.runner.Setenv("PATH", mergedPath); err != nil {
		return fmt.Errorf("failed to update the launcher PATH: %w", err)
	}

	return nil
}

func (preparer windowsHostEnvironmentPreparer) ensureDefaultMachine(
	ctx context.Context,
	options PrepareOptions,
) error {
	machines, err := preparer.runner.Output(ctx, hostCommand(
		"podman", "machine", "inspect", "--format", "{{.Name}}",
	))
	if err != nil {
		return err
	}
	if !containsLine(machines, windowsDefaultMachineName) {
		writePreparationProgress(options.Progress, "Creating a Podman machine...")
		if err := preparer.runner.Run(ctx, options.Progress, hostCommand(
			"podman", "machine", "init",
			"--disk-size", windowsMachineDiskSizeGB,
		)); err != nil {
			return err
		}

		return preparer.startDefaultMachine(ctx, options.Progress)
	}

	state, err := preparer.machineState(ctx)
	if err != nil {
		return err
	}

	if ! strings.EqualFold(state, "running") {
		if err := preparer.runner.Run(ctx, options.Progress, hostCommand(
			"podman", "machine", "start", windowsDefaultMachineName,
		)); err != nil {
			return err
		}
	}

	return nil
}

func (preparer windowsHostEnvironmentPreparer) machineState(ctx context.Context) (string, error) {
	return preparer.runner.Output(ctx, hostCommand(
		"podman", "machine", "inspect",
		"--format", "{{.State}}", windowsDefaultMachineName,
	))
}

func (preparer windowsHostEnvironmentPreparer) startDefaultMachine(
	ctx context.Context,
	progress io.Writer,
) error {
	writePreparationProgress(progress, "Starting the Podman machine...")

	return preparer.runner.Run(ctx, progress, hostCommand(
		"podman", "machine", "start", windowsDefaultMachineName,
	))
}

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

func hostCommand(name string, args ...string) HostCommand {
	return HostCommand{Name: name, Args: args}
}

func formatHostCommand(command HostCommand) string {
	return strings.TrimSpace(strings.Join(append([]string{command.Name}, command.Args...), " "))
}

func containsLine(value, expected string) bool {
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) == expected {
			return true
		}
	}

	return false
}

func mergeWindowsPath(current, registered string) string {
	seen := map[string]struct{}{}
	for _, entry := range strings.Split(current, ";") {
		if entry = strings.TrimSpace(entry); entry != "" {
			seen[strings.ToLower(entry)] = struct{}{}
		}
	}
	additions := make([]string, 0)
	for _, entry := range strings.Split(registered, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key := strings.ToLower(entry)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		additions = append(additions, entry)
	}
	if len(additions) == 0 {
		return current
	}
	if current == "" {
		return strings.Join(additions, ";")
	}

	return strings.Join(additions, ";") + ";" + current
}

func writePreparationProgress(writer io.Writer, message string) {
	if writer != nil {
		_, _ = fmt.Fprintln(writer, message)
	}
}
