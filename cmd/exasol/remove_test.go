// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestRemoveConfirmationPromptIncludesDeploymentDirectory(t *testing.T) {
	t.Parallel()

	prompt := removeConfirmationPrompt("/tmp/deployment")

	if !strings.Contains(prompt, "/tmp/deployment") {
		t.Fatalf("expected prompt to include deployment directory, got %q", prompt)
	}
}

func TestDestroyConfirmationPromptWithRemoveIncludesDeploymentDirectory(t *testing.T) {
	t.Parallel()

	prompt := destroyConfirmationPrompt("/tmp/deployment")

	if !strings.Contains(prompt, "/tmp/deployment") {
		t.Fatalf("expected prompt to include deployment directory, got %q", prompt)
	}
	if !strings.Contains(prompt, "remove after destroy") {
		t.Fatalf("expected prompt to mention removal-after-destroy, got %q", prompt)
	}
}

func TestDestroyConfirmationPromptWithoutRemoveOmitsDeploymentDirectory(t *testing.T) {
	t.Parallel()

	prompt := destroyConfirmationPrompt("")

	if strings.Contains(prompt, "/tmp/deployment") {
		t.Fatalf(
			"expected prompt to omit deployment directory when --remove is not set, got %q",
			prompt,
		)
	}
}
