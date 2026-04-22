// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"strings"
	"testing"
)

func TestAppendDeployFailureHint_AddsCloudResourceHint(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("tofu apply failed")

	// When
	err := appendDeployFailureHint(baseErr, backendTypeTofu)

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

func TestAppendDeployFailureHintNilInput(t *testing.T) {
	t.Parallel()

	// Given/When
	err := appendDeployFailureHint(nil, backendTypeTofu)
	// Then
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestAppendDeployFailureHint_AddsLocalLogHint(t *testing.T) {
	t.Parallel()

	// Given
	baseErr := errors.New("local runtime failed")

	// When
	err := appendDeployFailureHint(baseErr, backendTypeLocal)

	// Then
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "local-runtime/logs") {
		t.Fatalf("expected local log hint, got: %q", err.Error())
	}
}
