// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package vm

import "testing"

func TestClampCPUCount_UsesConfiguredBounds(t *testing.T) {
	t.Parallel()

	// Given
	minCPU := uint(2)
	maxCPU := uint(6)

	// When
	tooLow := clampCPUCount(0, minCPU, maxCPU)
	inRange := clampCPUCount(4, minCPU, maxCPU)
	tooHigh := clampCPUCount(8, minCPU, maxCPU)

	// Then
	if tooLow != minCPU {
		t.Fatalf("expected zero request to clamp to %d, got %d", minCPU, tooLow)
	}
	if inRange != 4 {
		t.Fatalf("expected in-range request to stay at 4, got %d", inRange)
	}
	if tooHigh != maxCPU {
		t.Fatalf("expected high request to clamp to %d, got %d", maxCPU, tooHigh)
	}
}

func TestClampMemoryBytes_AlignsAndClampsToMiB(t *testing.T) {
	t.Parallel()

	// Given
	minMemory := uint64(2 * mib)
	maxMemory := uint64(8 * mib)

	// When
	belowMinimum := clampMemoryBytes(mib/2, minMemory, maxMemory)
	inRange := clampMemoryBytes((5*mib)+17, minMemory, maxMemory)
	aboveMaximum := clampMemoryBytes(16*mib, minMemory, maxMemory)

	// Then
	if belowMinimum != minMemory {
		t.Fatalf("expected below-minimum request to clamp to %d, got %d", minMemory, belowMinimum)
	}
	if inRange != 5*mib {
		t.Fatalf("expected in-range request to align to %d, got %d", 5*mib, inRange)
	}
	if aboveMaximum != maxMemory {
		t.Fatalf("expected above-maximum request to clamp to %d, got %d", maxMemory, aboveMaximum)
	}
}

func TestResolvedSharedDirTag_PrefersExplicitTag(t *testing.T) {
	t.Parallel()

	// Given
	sharedDir := SharedDir{
		Tag:         "control-socket",
		Source:      "/tmp/control",
		Destination: "/exa/control",
	}

	// When
	tag := resolvedSharedDirTag(sharedDir, 0)

	// Then
	if tag != "control-socket" {
		t.Fatalf("expected explicit tag to be used, got %q", tag)
	}
}

func TestResolvedSharedDirTag_DerivesTagFromDestination(t *testing.T) {
	t.Parallel()

	// Given
	sharedDir := SharedDir{
		Source:      "/tmp/runtime-data",
		Destination: "/exa/runtime/control",
	}

	// When
	tag := resolvedSharedDirTag(sharedDir, 0)

	// Then
	if tag != "exa-runtime-control" {
		t.Fatalf("expected sanitized destination-derived tag, got %q", tag)
	}
}
