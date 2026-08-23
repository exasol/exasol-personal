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
	// ExtraRunArgs are appended to `podman run` before the image name.
	// Used on Windows rootless to inject pasta network flags that work around
	// the gvproxy multi-segment TLS handshake abort (see ROOTLESS_PODMAN_REPORT.md).
	ExtraRunArgs []string
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
