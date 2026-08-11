// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"io"
)

type StartConfig struct {
	ContainerDBPort      int
	DataDir              string
	InitParams           []string
	VersionCheck         VersionCheckConfig
	SLCs                 []SLCConfig
	LegacyContainerNames []string
}

type VersionCheckConfig struct {
	Enabled         bool
	URL             string
	Identity        string
	OperatingSystem string
	IntervalSeconds int
}

type SLCConfig struct {
	Image   string
	Target  string
	Package string
}

type InstallStatus struct {
	Running bool
}

type LocalInstall interface {
	// Prepare(ctx context.Context, out, outErr io.Writer) error
	Start(ctx context.Context, out, outErr io.Writer, config StartConfig) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	Status(ctx context.Context, out, outErr io.Writer) (*InstallStatus, error)
	Destroy(ctx context.Context, out, outErr io.Writer) error
}
