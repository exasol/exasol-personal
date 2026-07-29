// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

var windowsDrivePath = regexp.MustCompile(`^([A-Za-z]):[\\/](.*)$`)

type WindowsPodmanAdapter struct {
	BaseAdapter    BasePodmanAdapter
	DeploymentRoot string
	Commands       CommandRunner
}

func NewWindowsPodmanAdapter(deploymentRoot string, commands CommandRunner) *WindowsPodmanAdapter {
	if commands == nil {
		commands = OSCommandRunner{}
	}

	return &WindowsPodmanAdapter{
		BaseAdapter: BasePodmanAdapter{
			DeploymentRoot: deploymentRoot,
			Commands:       commands,
		},
		DeploymentRoot: deploymentRoot, Commands: commands}
}

func (*WindowsPodmanAdapter) Capabilities() RuntimeCapabilities {
	return PlatformCapabilities("windows")
}

func (adapter *WindowsPodmanAdapter) Prerequisites(
	ctx context.Context,
	options PrerequisiteOptions,
) error {
	wslStatus, err := adapter.Commands.Output(ctx, "wsl.exe", "--status")
	if err != nil {
		return fmt.Errorf("WSL2 is required for local deployments: %w", err)
	}
	if !reportsWSL2(wslStatus) {
		return errors.New(
			"WSL2 is required for local deployments; " +
				"`wsl.exe --status` does not report default version 2",
		)
	}
	if _, err := adapter.Commands.LookPath("podman"); err != nil {
		if err := requireInteractiveApproval(
			options,
			"Podman is required. Install it for the current user with Winget?",
		); err != nil {
			return err
		}
		if err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"winget",
			"install",
			"--id",
			"RedHat.Podman",
			"--exact",
			"--scope",
			"user",
			"--accept-package-agreements",
			"--accept-source-agreements",
		); err != nil {
			return fmt.Errorf("failed to install Podman with Winget: %w", err)
		}
	}
	if err := ensureWindowsMachine(ctx, adapter); err != nil {
		return err
	}
	versions, err := readPodmanVersions(ctx, adapter.Commands)
	if err != nil {
		return err
	}
	if versions.Client.LT(minimumPodmanVersion) ||
		versions.Server.LT(minimumPodmanVersion) {
		if err := requireInteractiveApproval(
			options,
			fmt.Sprintf(
				"Podman client/server %s/%s is below the validation minimum %s. "+
					"Upgrade it with Winget?",
				versions.Client,
				versions.Server,
				minimumPodmanVersion,
			),
		); err != nil {
			return err
		}
		if err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"winget",
			"upgrade",
			"--id",
			"RedHat.Podman",
			"--exact",
			"--scope",
			"user",
			"--accept-package-agreements",
			"--accept-source-agreements",
		); err != nil {
			return fmt.Errorf("failed to upgrade Podman with Winget: %w", err)
		}
		versions, err = readPodmanVersions(ctx, adapter.Commands)
		if err != nil {
			return fmt.Errorf("failed to verify Podman after Winget upgrade: %w", err)
		}
		if versions.Client.LT(minimumPodmanVersion) ||
			versions.Server.LT(minimumPodmanVersion) {
			return fmt.Errorf(
				"podman client/server remains below the validation minimum after upgrade: %s/%s",
				versions.Client,
				versions.Server,
			)
		}
	}

	return nil
}

func requireInteractiveApproval(options PrerequisiteOptions, prompt string) error {
	if !options.Interactive || options.Confirm == nil {
		return errors.New(prompt + " Re-run interactively to approve this user-scoped change")
	}
	approved, err := options.Confirm(prompt)
	if err != nil {
		return err
	}
	if !approved {
		return errors.New("podman prerequisite change was declined")
	}

	return nil
}

type podmanMachine struct {
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	Name string `json:"Name"`
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	Default bool `json:"Default"`
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	Running bool `json:"Running"`
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	VMType string `json:"VMType"`
}

func ensureWindowsMachine(ctx context.Context, adapter *WindowsPodmanAdapter) error {
	data, err := adapter.Commands.Output(ctx, "podman", "machine", "list", "--format", "json")
	if err != nil {
		return fmt.Errorf("failed to list Podman machines: %w", err)
	}
	var machines []podmanMachine
	if len(bytes.TrimSpace(data)) != 0 {
		if err := json.Unmarshal(data, &machines); err != nil {
			return fmt.Errorf("failed to parse Podman machine list: %w", err)
		}
	}
	if len(machines) == 0 {
		if err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"podman",
			"machine",
			"init",
		); err != nil {
			return fmt.Errorf("failed to initialize Podman machine: %w", err)
		}

		return adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"podman",
			"machine",
			"start",
		)
	}
	sort.SliceStable(machines, func(left, right int) bool {
		return machines[left].Default && !machines[right].Default
	})
	machine := machines[0]
	if machine.VMType != "" && !strings.EqualFold(machine.VMType, "wsl") {
		return fmt.Errorf("podman machine %q does not use the WSL2 provider", machine.Name)
	}
	if machine.Running {
		return nil
	}

	return adapter.Commands.Run(
		ctx,
		nil,
		io.Discard,
		io.Discard,
		"podman",
		"machine",
		"start",
		machine.Name,
	)
}

func (adapter *WindowsPodmanAdapter) Start(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) (*RuntimeStatus, error) {
	if err := os.MkdirAll(spec.DataPath, privateDirMode); err != nil {
		return nil, fmt.Errorf("failed to create Personal-owned data path: %w", err)
	}
	runtimeSpec, err := windowsRuntimeSpec(spec)
	if err != nil {
		return nil, err
	}

	return adapter.BaseAdapter.Start(ctx, runtimeSpec, stdout, stderr)
}

func (adapter *WindowsPodmanAdapter) Stop(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	runtimeSpec, err := windowsRuntimeSpec(spec)
	if err != nil {
		return err
	}

	return adapter.BaseAdapter.Stop(ctx, runtimeSpec, stdout, stderr)
}

func (adapter *WindowsPodmanAdapter) Status(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.BaseAdapter.Status(ctx, spec)
}

func (adapter *WindowsPodmanAdapter) Health(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.Status(ctx, spec)
}

func (adapter *WindowsPodmanAdapter) Logs(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.BaseAdapter.Logs(ctx, spec, stdout, stderr)
}

func (adapter *WindowsPodmanAdapter) Destroy(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.Stop(ctx, spec, stdout, stderr)
}

func (*WindowsPodmanAdapter) Shell(
	context.Context,
	WorkloadSpec,
	ShellKind,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return errors.New("VM and container shells are not supported on Windows local deployments")
}

func windowsRuntimeSpec(spec WorkloadSpec) (WorkloadSpec, error) {
	wslDataPath, err := WindowsPathToWSL(spec.DataPath)
	if err != nil {
		return WorkloadSpec{}, err
	}
	spec.DataPath = wslDataPath
	spec.DBHostAddress = "127.0.0.1"
	if err := spec.Validate(); err != nil {
		return WorkloadSpec{}, err
	}

	return spec, nil
}

func reportsWSL2(output []byte) bool {
	normalized := strings.ToLower(strings.ReplaceAll(string(output), "\x00", ""))
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ": 2") || strings.HasSuffix(line, ":\t2") {
			return true
		}
	}

	return false
}

func WindowsPathToWSL(path string) (string, error) {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("UNC path %q is not visible through the WSL drive mount", path)
	}
	matches := windowsDrivePath.FindStringSubmatch(path)
	if matches == nil {
		return "", fmt.Errorf("path %q must be an absolute Windows drive path", path)
	}
	drive := strings.ToLower(matches[1])
	remainder := strings.ReplaceAll(matches[2], `\`, "/")
	if remainder != "" &&
		(strings.HasPrefix(remainder, "/") ||
			strings.HasSuffix(remainder, "/") ||
			strings.Contains(remainder, "//")) {
		return "", fmt.Errorf("path %q is not canonical", path)
	}
	parts := strings.Split(remainder, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "":
			if remainder == "" {
				continue
			}

			return "", fmt.Errorf("path %q is not canonical", path)
		case ".", "..":
			return "", fmt.Errorf("path %q contains traversal", path)
		default:
			cleanParts = append(cleanParts, part)
		}
	}

	return "/mnt/" + drive + "/" + strings.Join(cleanParts, "/"), nil
}
