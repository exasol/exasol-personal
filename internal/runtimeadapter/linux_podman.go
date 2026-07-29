// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"context"
	"errors"
	"io"
)

var ErrLinuxPodmanNotImplemented = errors.New(
	"linux direct Podman local deployments are not implemented",
)

type LinuxPodmanAdapter struct{}

func (LinuxPodmanAdapter) Prerequisites(context.Context, PrerequisiteOptions) error {
	return ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Start(
	context.Context,
	WorkloadSpec,
	io.Writer,
	io.Writer,
) (*RuntimeStatus, error) {
	return nil, ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Stop(context.Context, WorkloadSpec, io.Writer, io.Writer) error {
	return ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Status(context.Context, WorkloadSpec) (*RuntimeStatus, error) {
	return nil, ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Health(context.Context, WorkloadSpec) (*RuntimeStatus, error) {
	return nil, ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Logs(context.Context, WorkloadSpec, io.Writer, io.Writer) error {
	return ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Destroy(context.Context, WorkloadSpec, io.Writer, io.Writer) error {
	return ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Shell(
	context.Context,
	WorkloadSpec,
	ShellKind,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return ErrLinuxPodmanNotImplemented
}

func (LinuxPodmanAdapter) Capabilities() RuntimeCapabilities {
	return RuntimeCapabilities{}
}
