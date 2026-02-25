// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"os"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

func expectNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func expectErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func mustContain(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Fatalf("expected output to contain %q, got: %s", substr, s)
	}
}

func TestPrepare_WritesVarsAndBinary(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{})

	expectNoErr(t, os.MkdirAll(cfg.WorkDir(), 0o700))

	// A minimal variables file with primitive types and defaults.
	variablesTF := `variable "region" {
  type = string
  default = "us-east-1"
}

variable "enabled" {
  type = bool
  default = true
}

variable "instance_count" {
  type = number
  default = 2
}
`

	//nolint:gosec // test data file
	expectNoErr(
		t,
		os.WriteFile(cfg.VariablesFile(), []byte(variablesTF), 0o644),
	)

	overrides := map[string]string{
		"region":         "eu-central-1",
		"enabled":        "false",
		"instance_count": "3",
		"extra":          "hello",
	}

	err := Prepare(cfg, overrides)
	expectNoErr(t, err)

	// Vars file should exist and contain our overrides
	data, err := os.ReadFile(cfg.VarsOutputFile())
	expectNoErr(t, err)
	out := string(data)
	mustContain(t, out, "region")
	mustContain(t, out, "eu-central-1")
	mustContain(t, out, "enabled")
	mustContain(t, out, "false")
	mustContain(t, out, "instance_count")
	mustContain(t, out, "3")
	mustContain(t, out, "extra")
	mustContain(t, out, "hello")

	// Embedded binary should exist.
	binPath := cfg.TofuBinaryPath()
	info, err := os.Stat(binPath)
	expectNoErr(t, err)
	if info.Size() == 0 {
		t.Fatalf("expected tofu binary at %q to be non-empty", binPath)
	}
}

func TestPrepare_ErrorsWhenVariablesMissing(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{})

	expectNoErr(t, os.MkdirAll(cfg.WorkDir(), 0o700))

	err := Prepare(cfg, nil)
	expectErr(t, err)
}
