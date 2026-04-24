// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"os"
	"testing"
)

func TestRuntimeLoadMachineSizing_WritesDefaultsWhenMissing(t *testing.T) {
	t.Parallel()

	runtime := New(t.TempDir())

	sizing, err := runtime.LoadMachineSizing()
	if err != nil {
		t.Fatalf("expected default sizing to load, got %v", err)
	}
	if sizing.CPUCount < minimumGuestCPUCount {
		t.Fatalf("expected cpu count >= %d, got %d", minimumGuestCPUCount, sizing.CPUCount)
	}
	if sizing.MemoryBytes != defaultGuestMemoryBytes {
		t.Fatalf("expected memory bytes %d, got %d", defaultGuestMemoryBytes, sizing.MemoryBytes)
	}
	if sizing.LayerDiskBytes != defaultGuestLayerDiskBytes {
		t.Fatalf(
			"expected layer disk bytes %d, got %d",
			defaultGuestLayerDiskBytes,
			sizing.LayerDiskBytes,
		)
	}
	if _, err := os.Stat(runtime.Layout().MachineSizingPath()); err != nil {
		t.Fatalf("expected machine sizing config to be written, got %v", err)
	}
}

func TestRuntimeLoadMachineSizing_NormalizesExistingConfig(t *testing.T) {
	t.Parallel()

	runtime := New(t.TempDir())
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected runtime root to be created, got %v", err)
	}
	if err := os.WriteFile(
		runtime.Layout().MachineSizingPath(),
		[]byte("{\"cpuCount\":1,\"memoryBytes\":0,\"layerDiskBytes\":0}\n"),
		0o600,
	); err != nil {
		t.Fatalf("expected machine sizing fixture to be written, got %v", err)
	}

	sizing, err := runtime.LoadMachineSizing()
	if err != nil {
		t.Fatalf("expected sizing to load, got %v", err)
	}
	if sizing.CPUCount < minimumGuestCPUCount {
		t.Fatalf("expected normalized cpu count, got %d", sizing.CPUCount)
	}
	if sizing.MemoryBytes != defaultGuestMemoryBytes {
		t.Fatalf(
			"expected normalized memory bytes %d, got %d",
			defaultGuestMemoryBytes,
			sizing.MemoryBytes,
		)
	}
	if sizing.LayerDiskBytes != defaultGuestLayerDiskBytes {
		t.Fatalf(
			"expected normalized layer disk bytes %d, got %d",
			defaultGuestLayerDiskBytes,
			sizing.LayerDiskBytes,
		)
	}
}
