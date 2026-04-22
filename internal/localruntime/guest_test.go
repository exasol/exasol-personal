// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
)

func TestRuntimePrepareGuest_BuildsMachineConfigFromSelectedRunPayload(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	runtime := New(deploymentDir)
	fixtureDir := t.TempDir()
	payloadPath := filepath.Join(fixtureDir, "exasol-nano-db-2026.1.0-arm64.run")
	kernelPath := filepath.Join(fixtureDir, "vmlinux.container")
	initrdPath := filepath.Join(fixtureDir, "ubuntu-initrd.cpio.gz")

	for path, content := range map[string]string{
		payloadPath: "runfile",
		kernelPath:  "kernel",
		initrdPath:  "initrd",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("expected fixture %q to be written, got %v", path, err)
		}
	}

	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
		Payload: &localstate.PayloadRef{
			Version:      "2026.1.0",
			Architecture: "arm64",
			Checksum:     "abc123",
			CachePath:    payloadPath,
			Boot: &localstate.PayloadBootRef{
				KernelPath: kernelPath,
				InitrdPath: initrdPath,
			},
		},
	}); err != nil {
		t.Fatalf("expected runtime state to be written, got %v", err)
	}

	// When
	guest, err := runtime.PrepareGuest(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected guest preparation to succeed, got %v", err)
	}
	if guest.Machine.KernelPath != kernelPath {
		t.Fatalf("expected kernel path %q, got %q", kernelPath, guest.Machine.KernelPath)
	}
	if guest.Machine.InitrdPath != initrdPath {
		t.Fatalf("expected initrd path %q, got %q", initrdPath, guest.Machine.InitrdPath)
	}
	if guest.Machine.DiskImage != runtime.Layout().LayerDiskImageFile() {
		t.Fatalf(
			"expected layer disk image %q, got %q",
			runtime.Layout().LayerDiskImageFile(),
			guest.Machine.DiskImage,
		)
	}
	if len(guest.Machine.SharedDirs) != 4 {
		t.Fatalf(
			"expected control, logs, payload, and provision shares, got %#v",
			guest.Machine.SharedDirs,
		)
	}
	if !strings.Contains(
		guest.Machine.KernelCommandLine,
		"exa_volume=exa-payload:/.exanano/payload",
	) {
		t.Fatalf(
			"expected command line to mount payload share, got %q",
			guest.Machine.KernelCommandLine,
		)
	}
	if !strings.Contains(
		guest.Machine.KernelCommandLine,
		"exa_volume=exa-provision:/.exanano/provision",
	) {
		t.Fatalf(
			"expected command line to mount provision share, got %q",
			guest.Machine.KernelCommandLine,
		)
	}

	bootstrapProfilePath := filepath.Join(runtime.Layout().BootstrapDir(), bootstrapProfileFileName)
	bootstrapEntrypointPath := filepath.Join(
		runtime.Layout().BootstrapDir(),
		entrypointWrapperFileName,
	)
	stagedPayloadPath := runtime.Layout().PayloadExecutablePath()

	if _, err := os.Stat(bootstrapProfilePath); err != nil {
		t.Fatalf("expected bootstrap profile to exist, got %v", err)
	}
	if _, err := os.Stat(bootstrapEntrypointPath); err != nil {
		t.Fatalf("expected bootstrap entrypoint to exist, got %v", err)
	}
	payloadInfo, err := os.Stat(stagedPayloadPath)
	if err != nil {
		t.Fatalf("expected staged payload to exist, got %v", err)
	}
	if payloadInfo.Mode()&0o111 == 0 {
		t.Fatalf("expected staged payload to be executable, got mode %v", payloadInfo.Mode())
	}
	if string(mustReadFile(t, stagedPayloadPath)) != "runfile" {
		t.Fatal("expected staged payload content to match source runfile")
	}
	layerInfo, err := os.Stat(runtime.Layout().LayerDiskImageFile())
	if err != nil {
		t.Fatalf("expected layer disk image to exist, got %v", err)
	}
	if layerInfo.Size() == 0 {
		t.Fatal("expected non-empty layer disk image")
	}
}

func TestRuntimePrepareGuest_FailsWithoutSelectedPayload(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())

	// When
	_, err := runtime.PrepareGuest(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing payload selection to fail")
	}
	if !errors.Is(err, ErrPayloadSelectionMissing) {
		t.Fatalf("expected payload selection missing error, got %v", err)
	}
}

func TestRuntimePrepareGuest_FailsWithoutBootAssets(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	payloadPath := filepath.Join(t.TempDir(), "exasol-nano-db.run")
	if err := os.WriteFile(payloadPath, []byte("runfile"), 0o600); err != nil {
		t.Fatalf("expected payload fixture to be written, got %v", err)
	}
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
		Payload: &localstate.PayloadRef{
			Version:      "2026.1.0",
			Architecture: "arm64",
			CachePath:    payloadPath,
		},
	}); err != nil {
		t.Fatalf("expected runtime state to be saved, got %v", err)
	}

	// When
	_, err := runtime.PrepareGuest(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing boot assets to fail")
	}
	if !errors.Is(err, ErrPayloadBootAssetsMissing) {
		t.Fatalf("expected missing boot assets error, got %v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %q to be readable, got %v", path, err)
	}

	return data
}
