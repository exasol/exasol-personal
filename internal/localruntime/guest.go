// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

const (
	localRuntimeDirMode      = 0o700
	localRuntimeFileMode     = 0o600
	localRuntimeExecFileMode = 0o700
)

var ErrPayloadSelectionMissing = errors.New("local runtime payload selection is missing")

type GuestConfig struct {
	Controller Controller
	Machine    vm.MachineConfig
}

func (r *Runtime) PrepareGuest(ctx context.Context) (*GuestConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.EnsureRoot(); err != nil {
		return nil, err
	}

	state, err := r.LoadState()
	if err != nil {
		return nil, err
	}
	if state.Payload == nil ||
		strings.TrimSpace(state.Payload.DiskImagePath) == "" ||
		strings.TrimSpace(state.Payload.RunPath) == "" {
		return nil, ErrPayloadSelectionMissing
	}

	sizing, err := r.LoadMachineSizing()
	if err != nil {
		return nil, err
	}

	stagedDiskPath, err := r.StageDiskImage(state.Payload.DiskImagePath)
	if err != nil {
		return nil, err
	}
	if err := r.StagePayloadShare(state.Payload.RunPath); err != nil {
		return nil, err
	}

	machineConfig := vm.MachineConfig{
		Name:                  deploymentMachineName(r.layout.DeploymentDir()),
		DiskImagePath:         stagedDiskPath,
		EFIVarsPath:           r.layout.EFIVarsPath(),
		CPUCount:              sizing.CPUCount,
		MemoryBytes:           sizing.MemoryBytes,
		MachineIdentifierPath: r.layout.MachineIdentifierFile(),
		ConsoleLogPath:        r.layout.ConsoleLogFile(),
		SharedDirs: []vm.SharedDir{{
			Tag:         guestPayloadShareTag,
			Source:      r.layout.PayloadShareDir(),
			Destination: guestPayloadShareMount,
			ReadOnly:    false,
		}},
	}

	return &GuestConfig{
		Controller: r.Controller(),
		Machine:    machineConfig,
	}, nil
}

func deploymentMachineName(deploymentDir string) string {
	name := filepath.Base(strings.TrimSpace(deploymentDir))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "local-exasol"
	}

	return name
}
