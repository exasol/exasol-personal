// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

var ErrLinuxPodmanNotImplemented = errors.New(
	"linux direct Podman local deployments are not implemented",
)

type LinuxPodmanAdapter struct {
	BaseAdapter    BasePodmanAdapter
	DeploymentRoot string
	Commands       CommandRunner
}

func NewLinuxPodmanAdapter(deploymentRoot string, commands CommandRunner) *LinuxPodmanAdapter {
	if commands == nil {
		commands = OSCommandRunner{}
	}

	return &LinuxPodmanAdapter{
		BaseAdapter: BasePodmanAdapter{
			DeploymentRoot: deploymentRoot,
			Commands:       commands,
		},
		DeploymentRoot: deploymentRoot, Commands: commands}
}

func (adapter *LinuxPodmanAdapter) Prerequisites(ctx context.Context,
	options PrerequisiteOptions,
) error {
	if _, err := adapter.Commands.LookPath("podman"); err != nil {
		return fmt.Errorf("Podman is required: %w.", err)
	}
	versions, err := readPodmanVersions(ctx, adapter.Commands)
	if err != nil {
		return err
	}
	if versions.Client.LT(minimumPodmanVersion) ||
		versions.Server.LT(minimumPodmanVersion) {
		return fmt.Errorf(
			"Podman client/server %s/%s is below the validation minimum %s.",
			versions.Client,
			versions.Server,
			minimumPodmanVersion,
		)
	}

	return nil
}

func (adapter *LinuxPodmanAdapter) Start(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) (*RuntimeStatus, error) {
	if err := os.MkdirAll(spec.DataPath, privateDirMode); err != nil {
		return nil, fmt.Errorf("failed to create Personal-owned data path: %w", err)
	}

	return adapter.BaseAdapter.Start(ctx, spec, stdout, stderr)
}

func (adapter *LinuxPodmanAdapter) Stop(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.BaseAdapter.Stop(ctx, spec, stdout, stderr)
}

func (adapter *LinuxPodmanAdapter) Status(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.BaseAdapter.Status(ctx, spec)
}

func (adapter *LinuxPodmanAdapter) Health(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.Status(ctx, spec)
}

func (adapter *LinuxPodmanAdapter) Logs(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.BaseAdapter.Logs(ctx, spec, stdout, stderr)
}

func (adapter *LinuxPodmanAdapter) Destroy(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.Stop(ctx, spec, stdout, stderr)
}

func (*LinuxPodmanAdapter) Shell(
	context.Context,
	WorkloadSpec,
	ShellKind,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return errors.New("VM and container shells are not supported on Linux local deployments")
}

func (*LinuxPodmanAdapter) Capabilities() RuntimeCapabilities {
	return PlatformCapabilities("linux")
}
