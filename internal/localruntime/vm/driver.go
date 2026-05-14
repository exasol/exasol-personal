// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package vm

import (
	"context"
	"errors"
)

var (
	ErrUnsupportedPlatform    = errors.New("local VM driver is unsupported on this platform")
	ErrNotImplemented         = errors.New("local VM driver is not implemented")
	ErrAlreadyRunning         = errors.New("local VM driver is already running")
	ErrPortForwardUnsupported = errors.New(
		"local VM driver does not provide host port forwarding",
	)
)

type SharedDir struct {
	Tag         string
	Source      string
	Destination string
	ReadOnly    bool
}

type PortForward struct {
	Name      string
	HostPort  int
	GuestPort int
}

// BootMode selects how the guest is booted.
type BootMode int

const (
	// BootModeLinux direct-boots a kernel + initrd with a kernel command line.
	BootModeLinux BootMode = iota
	// BootModeEFI boots a UEFI disk image and uses a persistent EFI variable store.
	BootModeEFI
)

type MachineConfig struct {
	Name                  string
	BootMode              BootMode
	KernelPath            string
	InitrdPath            string
	KernelCommandLine     string
	EFIVariableStorePath  string
	DiskImage             string
	CPUCount              int
	MemoryBytes           uint64
	MachineIdentifierPath string
	ConsoleLogPath        string
	SharedDirs            []SharedDir
	PortForwards          []PortForward
}

type Driver interface {
	Start(ctx context.Context, config MachineConfig) error
	Stop(ctx context.Context) error
	Wait(ctx context.Context) error
	Running() bool
}
