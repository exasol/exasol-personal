// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
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
	MACAddress string
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

	mac, err := generateLocallyAdministeredMAC()
	if err != nil {
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
		MACAddress:            mac,
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
		MACAddress: mac,
	}, nil
}

// generateLocallyAdministeredMAC produces a random MAC with the
// locally-administered bit set and the multicast bit cleared so the
// resulting address is valid for unicast frames on a private bridge.
func generateLocallyAdministeredMAC() (string, error) {
	const (
		// IEEE 802 first-octet bits.
		locallyAdministeredBit byte = 0x02
		multicastBit           byte = 0x01
	)

	var bytes [6]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("failed to generate MAC address: %w", err)
	}
	bytes[0] = (bytes[0] | locallyAdministeredBit) &^ multicastBit

	return fmt.Sprintf(
		"%02x:%02x:%02x:%02x:%02x:%02x",
		bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5],
	), nil
}

func deploymentMachineName(deploymentDir string) string {
	name := filepath.Base(strings.TrimSpace(deploymentDir))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "local-exasol"
	}

	return name
}
