// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"regexp"
	"testing"
)

func TestGenerateDatabasePassword_FormatAndUniqueness(t *testing.T) {
	t.Parallel()

	// Given
	pattern := regexp.MustCompile(`^[A-Za-z0-9]{16}$`)
	seen := map[string]bool{}

	for range 100 {
		// When
		password, err := generateDatabasePassword()
		// Then
		if err != nil {
			t.Fatalf("generateDatabasePassword failed: %v", err)
		}
		if !pattern.MatchString(password) {
			t.Fatalf("unexpected database password format %q", password)
		}
		if seen[password] {
			t.Fatalf("generated duplicate database password %q", password)
		}
		seen[password] = true
	}
}

func TestGenerateDeploymentId_Format(t *testing.T) {
	t.Parallel()

	deploymentId, err := GenerateDeploymentId()
	if err != nil {
		t.Fatalf("GenerateDeploymentId failed: %v", err)
	}

	// Note: hex strings can be digits-only (e.g. "24245818") and still be valid.
	pattern := regexp.MustCompile(`^[0-9a-f]{8}$`)
	if !pattern.MatchString(deploymentId) {
		t.Fatalf("unexpected deployment id %q", deploymentId)
	}
}
