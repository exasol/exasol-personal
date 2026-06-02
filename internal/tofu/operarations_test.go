// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"os"
	"path/filepath"
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

func TestPrepare_WritesVars(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{})

	expectNoErr(t, os.MkdirAll(cfg.WorkDir(), 0o700))

	writeMinimalVariablesFile(t, cfg)

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
}

func TestConfigure_WritesVarsWithoutBinary(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{})
	binaryPath := filepath.Join(deploymentDir, "tofu")
	cfg.tofuBinaryPath = binaryPath

	expectNoErr(t, os.MkdirAll(cfg.WorkDir(), 0o700))
	writeMinimalVariablesFile(t, cfg)

	err := Configure(cfg, map[string]string{"region": "eu-west-1"})
	expectNoErr(t, err)

	data, err := os.ReadFile(cfg.VarsOutputFile())
	expectNoErr(t, err)
	mustContain(t, string(data), "eu-west-1")

	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Fatalf("expected configure not to write tofu binary, got: %v", err)
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

func writeMinimalVariablesFile(t *testing.T, cfg *Config) {
	t.Helper()

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
}
