// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"io"

	"github.com/exasol/exasol-personal/internal/config"
)

type VMConfig struct {
	CPUCount   int
	MemoryMB   int
	DataSizeGB int
	Ports      string
}

type VMStatus struct {
	Running bool `json:"running"`
}

// Generic local runtime interface.
type Runtime interface {
	Deployment() config.DeploymentDir
	Prepare(ctx context.Context, out, outErr io.Writer) error
	Start(ctx context.Context, out, outErr io.Writer, runtimeConfig VMConfig) error
	Stop(ctx context.Context, out, outErr io.Writer) error
	Status(ctx context.Context) (*VMStatus, error)
	Destroy(ctx context.Context, out, outErr io.Writer) error
}

// VM-based local runtime interface.
type VMRuntime interface {
	Runtime

	ReadState() (*State, error)
	HealthCheck(ctx context.Context) (*HealthCheckResult, error)
}
