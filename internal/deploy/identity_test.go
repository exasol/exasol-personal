// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"regexp"
	"testing"
)

func TestPresetIdentityToken_NameBasedRef(t *testing.T) {
	t.Parallel()

	if got := presetIdentityToken(PresetRef{Name: "  aws  "}); got != "aws" {
		t.Fatalf("expected trimmed name token, got %q", got)
	}
}

func TestPresetIdentityToken_PathBasedRefUsesBaseName(t *testing.T) {
	t.Parallel()

	got := presetIdentityToken(PresetRef{Path: "/tmp/my presets/custom-infra/"})
	if got != "custom-infra" {
		t.Fatalf("expected the cleaned path's base name, got %q", got)
	}
}

func TestPresetIdentityToken_BlankTokenFallsBackToUnknown(t *testing.T) {
	t.Parallel()

	const unknownToken = "unknown"

	if got := presetIdentityToken(PresetRef{Name: "   "}); got != unknownToken {
		t.Fatalf("expected %q for a blank preset name, got %q", unknownToken, got)
	}
}

func TestPresetIdentityToken_ReplacesDelimitersAndSpaces(t *testing.T) {
	t.Parallel()

	got := presetIdentityToken(PresetRef{Name: "my;custom infra"})
	if got != "my_custom_infra" {
		t.Fatalf("expected delimiter-safe token, got %q", got)
	}
}

func TestComputeClusterIdentity_JoinsAllComponents(t *testing.T) {
	t.Parallel()

	got := ComputeClusterIdentity(
		"abcd1234",
		PresetRef{Name: "aws"},
		PresetRef{Name: "ubuntu"},
	)
	want := "exasol-personal;abcd1234;aws;ubuntu"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestComputeClusterIdentity_BlankDeploymentIdFallsBackToUnknown(t *testing.T) {
	t.Parallel()

	got := ComputeClusterIdentity("  ", PresetRef{Name: "aws"}, PresetRef{Name: "ubuntu"})
	want := "exasol-personal;unknown;aws;ubuntu"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
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
