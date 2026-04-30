// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package vm

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedDriverReturnsPlatformError(t *testing.T) {
	t.Parallel()

	// Given
	driver := New()

	// When
	err := driver.Start(context.Background(), MachineConfig{Name: "test"})

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnsupportedPlatform) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected unsupported or not implemented error, got %v", err)
	}
}

func TestValidateMachineConfig_RequiresName(t *testing.T) {
	t.Parallel()

	err := validateMachineConfig(MachineConfig{
		DiskImagePath: "/tmp/disk.img",
		EFIVarsPath:   "/tmp/efi-vars.fd",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestValidateMachineConfig_RequiresDiskImagePath(t *testing.T) {
	t.Parallel()

	err := validateMachineConfig(MachineConfig{
		Name:        "test",
		EFIVarsPath: "/tmp/efi-vars.fd",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestValidateMachineConfig_RequiresEFIVarsPath(t *testing.T) {
	t.Parallel()

	err := validateMachineConfig(MachineConfig{
		Name:          "test",
		DiskImagePath: "/tmp/disk.img",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestValidateMachineConfig_RejectsPortForwards(t *testing.T) {
	t.Parallel()

	err := validateMachineConfig(MachineConfig{
		Name:          "test",
		DiskImagePath: "/tmp/disk.img",
		EFIVarsPath:   "/tmp/efi-vars.fd",
		PortForwards: []PortForward{
			{Name: "db", HostPort: 1234, GuestPort: 5678},
		},
	})
	if !errors.Is(err, ErrPortForwardUnsupported) {
		t.Fatalf("expected ErrPortForwardUnsupported, got %v", err)
	}
}

func TestValidateMachineConfig_AcceptsValidConfig(t *testing.T) {
	t.Parallel()

	err := validateMachineConfig(MachineConfig{
		Name:          "test",
		DiskImagePath: "/tmp/disk.img",
		EFIVarsPath:   "/tmp/efi-vars.fd",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
