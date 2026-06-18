// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"context"
	"runtime"
	"testing"
)

func TestGetTotalMemoryMB(t *testing.T) {
	t.Parallel()

	memoryMB, err := GetTotalMemoryMB(context.Background())

	if runtime.GOOS == "darwin" {
		// macOS is the only implemented platform; it should report real memory.
		if err != nil {
			t.Fatalf("expected darwin host memory, got error: %v", err)
		}
		if memoryMB == 0 {
			t.Fatal("expected non-zero darwin host memory")
		}

		return
	}

	// Other platforms are not implemented yet and must report an error with no value.
	if err == nil {
		t.Fatalf(
			"expected unsupported-platform error on %s, got nil (value %d)",
			runtime.GOOS,
			memoryMB,
		)
	}
	if memoryMB != 0 {
		t.Fatalf("expected zero memory on unsupported platform, got %d", memoryMB)
	}
}
