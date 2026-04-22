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
