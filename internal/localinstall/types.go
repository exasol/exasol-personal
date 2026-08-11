// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"io"
)

type StartConfig struct {
	HostDBPort      int
	ContainerDBPort int
	DataDir         string
	ShmSize         string
	PIDsLimit       string
	SecurityOpt     string
	RestartPolicy   string
	InitParams      []string
}

type LocalInstall interface {
	// Prepare(ctx context.Context, out, outErr io.Writer) error
	Start(ctx context.Context, out, outErr io.Writer, config StartConfig) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	// Status(ctx context.Context, out, outErr io.Writer) (*RuntimeStatus, error)
	Destroy(ctx context.Context, out, outErr io.Writer) error
}
