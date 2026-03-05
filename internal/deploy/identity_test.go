// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"regexp"
	"testing"
)

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
