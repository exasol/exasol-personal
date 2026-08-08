// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/presets"
)

// withFakeTofuBinary swaps execCommandContext so the real tofu binary is
// never invoked, and reports the args each Init/Plan/Apply/Destroy call
// would have executed it with.
func withFakeTofuBinary(t *testing.T) *[]string {
	t.Helper()

	var gotArgs []string
	orig := execCommandContext
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{name}, args...)

		return exec.CommandContext(ctx, "true")
	}
	t.Cleanup(func() { execCommandContext = orig })

	return &gotArgs
}

func fakeTofuOperationsConfig() *Config {
	return &Config{
		workDir:        "/tmp",
		tofuBinaryPath: "/bin/tofu",
		varsOutputFile: "/vars.tfvars",
		planeFile:      "/plan.tfplan",
		stateFile:      "/state.tfstate",
	}
}

// nolint: paralleltest
func TestInitialize_RunsTofuInitWithConfiguredLockfileMode(t *testing.T) {
	gotArgs := withFakeTofuBinary(t)

	err := Initialize(
		context.Background(),
		fakeTofuOperationsConfig(),
		io.Discard,
		io.Discard,
		LockfileUpdate,
	)
	if err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}

	want := []string{"/bin/tofu", "init", "-lockfile=update"}
	if strings.Join(*gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, *gotArgs)
	}
}

// nolint: paralleltest
func TestPlan_RunsTofuPlanWithConfiguredPaths(t *testing.T) {
	gotArgs := withFakeTofuBinary(t)

	err := Plan(context.Background(), fakeTofuOperationsConfig(), io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("Plan() returned error: %v", err)
	}

	want := []string{
		"/bin/tofu", "plan", "-out=/plan.tfplan", "-var-file=/vars.tfvars",
		"-state=/state.tfstate", "-state-out=/state.tfstate",
	}
	if strings.Join(*gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, *gotArgs)
	}
}

// nolint: paralleltest
func TestApplyPlan_RunsTofuApplyFromThePlanFileOnly(t *testing.T) {
	gotArgs := withFakeTofuBinary(t)

	err := ApplyPlan(context.Background(), fakeTofuOperationsConfig(), io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("ApplyPlan() returned error: %v", err)
	}

	want := []string{
		"/bin/tofu",
		"apply",
		"--auto-approve",
		"-state=/state.tfstate",
		"/plan.tfplan",
	}
	if strings.Join(*gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, *gotArgs)
	}
}

// nolint: paralleltest
func TestApplyAction_RunsTofuApplyWithActionVarAndVarsFile(t *testing.T) {
	gotArgs := withFakeTofuBinary(t)

	cfg := fakeTofuOperationsConfig()
	err := ApplyAction(context.Background(), cfg, "instance_action=Stop", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("ApplyAction() returned error: %v", err)
	}

	want := []string{
		"/bin/tofu", "apply", "--auto-approve", "-var=instance_action=Stop",
		"-var-file=/vars.tfvars", "-state=/state.tfstate",
	}
	if strings.Join(*gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, *gotArgs)
	}
}

// nolint: paralleltest
func TestDestroy_RunsTofuDestroyWithConfiguredPaths(t *testing.T) {
	gotArgs := withFakeTofuBinary(t)

	err := Destroy(context.Background(), fakeTofuOperationsConfig(), io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("Destroy() returned error: %v", err)
	}

	want := []string{
		"/bin/tofu", "destroy", "-var-file=/vars.tfvars", "--auto-approve", "-state=/state.tfstate",
	}
	if strings.Join(*gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, *gotArgs)
	}
}

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
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{}, nil)

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
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{}, nil)
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
	cfg := NewTofuConfigFromDeployment(deploymentDir, presets.InfrastructureTofu{}, nil)

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
