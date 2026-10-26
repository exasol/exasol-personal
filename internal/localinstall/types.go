// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"io"
)

type StartConfig struct {
	ContainerDBPort      int
	ContainerDBBindHost  string
	DataDir              string
	InitParams           []string
	VersionCheck         VersionCheckConfig
	SLCs                 []SLCConfig
	LegacyContainerNames []string
	// BootstrapDir receives the initial sys password for a fresh data directory
	// and is mounted read-only into that first container only.
	BootstrapDir string
	// InitialPassword is called only when the data directory is fresh.
	InitialPassword func(context.Context) (string, error)
	// PrepareDataDir, when set, adapts the data directory to the container
	// engine. It runs once the directory's final location is settled, after any
	// recovery or migration has replaced it, and before the container is created.
	PrepareDataDir func(ctx context.Context, dataDir string) error
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
	// Running includes a container its engine is restarting or has paused,
	// because the engine brings it back without the launcher.
	Running bool
}

type LocalInstall interface {
	// Prepare(ctx context.Context, out, outErr io.Writer) error
	Start(ctx context.Context, out, outErr io.Writer, config StartConfig) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	Status(ctx context.Context, out, outErr io.Writer) (*InstallStatus, error)
	Destroy(ctx context.Context, out, outErr io.Writer) error
}
