// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
)

func TestRuntimePrepareGuest_BuildsMachineConfigFromSelectedAssets(t *testing.T) {
	t.Parallel()

	// Given
	deploymentDir := t.TempDir()
	runtime := New(deploymentDir)
	cacheDir := t.TempDir()
	cachedDiskPath := filepath.Join(cacheDir, "exasol-nano-vm.img")
	cachedRunPath := filepath.Join(cacheDir, "exasol-nano-db.run")
	if err := os.WriteFile(cachedDiskPath, []byte("disk-image"), 0o600); err != nil {
		t.Fatalf("expected disk image fixture to be written, got %v", err)
	}
	if err := os.WriteFile(cachedRunPath, []byte("run-binary"), 0o600); err != nil {
		t.Fatalf("expected run binary fixture to be written, got %v", err)
	}

	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
		Payload: &localstate.PayloadRef{
			Version:       "2026.1.0",
			Architecture:  "arm64",
			Checksum:      "abc123",
			DiskImagePath: cachedDiskPath,
			RunPath:       cachedRunPath,
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
	if guest.Machine.DiskImagePath != runtime.Layout().DiskImagePath() {
		t.Fatalf(
			"expected disk image path %q, got %q",
			runtime.Layout().DiskImagePath(),
			guest.Machine.DiskImagePath,
		)
	}
	if guest.Machine.EFIVarsPath != runtime.Layout().EFIVarsPath() {
		t.Fatalf(
			"expected EFI vars path %q, got %q",
			runtime.Layout().EFIVarsPath(),
			guest.Machine.EFIVarsPath,
		)
	}
	if len(guest.Machine.SharedDirs) != 1 {
		t.Fatalf(
			"expected exactly one shared dir, got %#v",
			guest.Machine.SharedDirs,
		)
	}
	share := guest.Machine.SharedDirs[0]
	if share.Tag != guestPayloadShareTag {
		t.Fatalf("expected share tag %q, got %q", guestPayloadShareTag, share.Tag)
	}
	if share.Source != runtime.Layout().PayloadShareDir() {
		t.Fatalf(
			"expected share source %q, got %q",
			runtime.Layout().PayloadShareDir(),
			share.Source,
		)
	}
	if share.Destination != guestPayloadShareMount {
		t.Fatalf(
			"expected share destination %q, got %q",
			guestPayloadShareMount,
			share.Destination,
		)
	}
	if share.ReadOnly {
		t.Fatal("expected payload share to be read-write")
	}
	if _, err := os.Stat(guest.Machine.DiskImagePath); err != nil {
		t.Fatalf("expected staged disk image to exist, got %v", err)
	}
	stagedRun := runtime.Layout().PayloadRunPath()
	stagedRunInfo, err := os.Stat(stagedRun)
	if err != nil {
		t.Fatalf("expected staged run binary to exist, got %v", err)
	}
	if stagedRunInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected staged run binary to be executable, got mode %v", stagedRunInfo.Mode())
	}
	if string(mustReadFile(t, stagedRun)) != "run-binary" {
		t.Fatal("expected staged run binary content to match cache")
	}
	stagedStart := runtime.Layout().PayloadStartScriptPath()
	stagedStartInfo, err := os.Stat(stagedStart)
	if err != nil {
		t.Fatalf("expected staged start script to exist, got %v", err)
	}
	if stagedStartInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf(
			"expected staged start script to be executable, got mode %v",
			stagedStartInfo.Mode(),
		)
	}
	if _, err := os.Stat(runtime.Layout().MachineSizingPath()); err != nil {
		t.Fatalf("expected machine sizing config to exist, got %v", err)
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

func TestRuntimePrepareGuest_FailsWithoutDiskImagePath(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
		Payload: &localstate.PayloadRef{
			Version:      "2026.1.0",
			Architecture: "arm64",
			RunPath:      "/some/run",
		},
	}); err != nil {
		t.Fatalf("expected runtime state to be saved, got %v", err)
	}

	// When
	_, err := runtime.PrepareGuest(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing disk image path to fail")
	}
	if !errors.Is(err, ErrPayloadSelectionMissing) {
		t.Fatalf("expected payload selection missing error, got %v", err)
	}
}

func TestRuntimePrepareGuest_FailsWithoutRunPath(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 8563,
			"ui": 8443,
		},
		Payload: &localstate.PayloadRef{
			Version:       "2026.1.0",
			Architecture:  "arm64",
			DiskImagePath: "/some/disk",
		},
	}); err != nil {
		t.Fatalf("expected runtime state to be saved, got %v", err)
	}

	// When
	_, err := runtime.PrepareGuest(context.Background())

	// Then
	if err == nil {
		t.Fatal("expected missing run path to fail")
	}
	if !errors.Is(err, ErrPayloadSelectionMissing) {
		t.Fatalf("expected payload selection missing error, got %v", err)
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
