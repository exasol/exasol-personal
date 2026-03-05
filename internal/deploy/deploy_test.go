// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"strings"
	"testing"
)

func TestAppendDeployFailureResourceHint(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("tofu apply failed")

	// When
	err := appendDeployFailureResourceHint(baseErr)

	// Then
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("expected wrapped error to match base error, got: %v", err)
	}
	if !strings.Contains(err.Error(), deployFailureResourceHint) {
		t.Fatalf("expected error to include resource hint, got: %q", err.Error())
	}
}

func TestAppendDeployFailureResourceHintNilInput(t *testing.T) {
	t.Parallel()

	// Given/When
	err := appendDeployFailureResourceHint(nil)
	// Then
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}
