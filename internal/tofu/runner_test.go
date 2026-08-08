// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"
	"io"
	"os/exec"
	"reflect"
	"testing"
)

// nolint: paralleltest
func TestTofuRunnerInit_UsesReadonlyLockfileByDefault(t *testing.T) {
	testTofuRunnerInitArgs(t, LockfileReadonly, []string{"/bin/tofu", "init", "-lockfile=readonly"})
}

// nolint: paralleltest
func TestTofuRunnerInit_UsesUpdateLockfileWhenEnabled(t *testing.T) {
	testTofuRunnerInitArgs(t, LockfileUpdate, []string{"/bin/tofu", "init", "-lockfile=update"})
}

// nolint: paralleltest
func TestTofuRunnerPlan_BuildsArgsWithAndWithoutState(t *testing.T) {
	cases := map[string]struct {
		stateFilePath string
		wantArgs      []string
	}{
		"without state": {
			stateFilePath: "",
			wantArgs: []string{
				"/bin/tofu",
				"plan",
				"-out=/plan.tfplan",
				"-var-file=/vars.tfvars",
			},
		},
		"with state": {
			stateFilePath: "/state.tfstate",
			wantArgs: []string{
				"/bin/tofu", "plan", "-out=/plan.tfplan", "-var-file=/vars.tfvars",
				"-state=/state.tfstate", "-state-out=/state.tfstate",
			},
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			var gotArgs []string
			orig := execCommandContext
			execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				gotArgs = append([]string{name}, args...)

				return exec.CommandContext(ctx, "true")
			}
			defer func() { execCommandContext = orig }()

			runner, err := NewTofuRunner(
				ctx,
				&Config{workDir: "/tmp", tofuBinaryPath: "/bin/tofu"},
				io.Discard,
				io.Discard,
			)
			if err != nil {
				t.Fatalf("NewTofuRunner() returned error: %v", err)
			}

			err = runner.Plan(ctx, "/plan.tfplan", "/vars.tfvars", testCase.stateFilePath)
			if err != nil {
				t.Fatalf("Plan() returned error: %v", err)
			}

			if !reflect.DeepEqual(gotArgs, testCase.wantArgs) {
				t.Fatalf(
					"%s: unexpected args.\nwant: %v\ngot:  %v",
					name,
					testCase.wantArgs,
					gotArgs,
				)
			}
		})
	}
}

// nolint: paralleltest
func TestTofuRunnerApply_BuildsArgsFromOptions(t *testing.T) {
	cases := map[string]struct {
		opts     ApplyOptions
		wantArgs []string
	}{
		"plan file, no vars (start/stop)": {
			opts: ApplyOptions{PlanFilePath: "/plan.tfplan", StateFilePath: "/state.tfstate"},
			wantArgs: []string{
				"/bin/tofu",
				"apply",
				"--auto-approve",
				"-state=/state.tfstate",
				"/plan.tfplan",
			},
		},
		"vars and var-args, no plan file": {
			opts: ApplyOptions{
				VarArgs:       []string{"instance_action=Stop"},
				VarsFilePath:  "/vars.tfvars",
				StateFilePath: "/state.tfstate",
			},
			wantArgs: []string{
				"/bin/tofu", "apply", "--auto-approve", "-var=instance_action=Stop",
				"-var-file=/vars.tfvars", "-state=/state.tfstate",
			},
		},
		"empty var-args entries are skipped": {
			opts:     ApplyOptions{VarArgs: []string{"", "keep=me", ""}},
			wantArgs: []string{"/bin/tofu", "apply", "--auto-approve", "-var=keep=me"},
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			var gotArgs []string
			orig := execCommandContext
			execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				gotArgs = append([]string{name}, args...)

				return exec.CommandContext(ctx, "true")
			}
			defer func() { execCommandContext = orig }()

			runner, err := NewTofuRunner(
				ctx,
				&Config{workDir: "/tmp", tofuBinaryPath: "/bin/tofu"},
				io.Discard,
				io.Discard,
			)
			if err != nil {
				t.Fatalf("NewTofuRunner() returned error: %v", err)
			}

			if err := runner.Apply(ctx, testCase.opts); err != nil {
				t.Fatalf("Apply() returned error: %v", err)
			}

			if !reflect.DeepEqual(gotArgs, testCase.wantArgs) {
				t.Fatalf(
					"%s: unexpected args.\nwant: %v\ngot:  %v",
					name,
					testCase.wantArgs,
					gotArgs,
				)
			}
		})
	}
}

// nolint: paralleltest
func TestTofuRunnerDestroy_BuildsArgsWithAndWithoutState(t *testing.T) {
	cases := map[string]struct {
		stateFilePath string
		wantArgs      []string
	}{
		"without state": {
			stateFilePath: "",
			wantArgs: []string{
				"/bin/tofu",
				"destroy",
				"-var-file=/vars.tfvars",
				"--auto-approve",
			},
		},
		"with state": {
			stateFilePath: "/state.tfstate",
			wantArgs: []string{
				"/bin/tofu", "destroy", "-var-file=/vars.tfvars", "--auto-approve",
				"-state=/state.tfstate",
			},
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			var gotArgs []string
			orig := execCommandContext
			execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				gotArgs = append([]string{name}, args...)

				return exec.CommandContext(ctx, "true")
			}
			defer func() { execCommandContext = orig }()

			runner, err := NewTofuRunner(
				ctx,
				&Config{workDir: "/tmp", tofuBinaryPath: "/bin/tofu"},
				io.Discard,
				io.Discard,
			)
			if err != nil {
				t.Fatalf("NewTofuRunner() returned error: %v", err)
			}

			err = runner.Destroy(ctx, "/vars.tfvars", testCase.stateFilePath)
			if err != nil {
				t.Fatalf("Destroy() returned error: %v", err)
			}

			if !reflect.DeepEqual(gotArgs, testCase.wantArgs) {
				t.Fatalf(
					"%s: unexpected args.\nwant: %v\ngot:  %v",
					name,
					testCase.wantArgs,
					gotArgs,
				)
			}
		})
	}
}

func testTofuRunnerInitArgs(t *testing.T, lockfileMode LockfileMode, wantArgs []string) {
	t.Helper()

	ctx := context.Background()

	var gotArgs []string
	orig := execCommandContext
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{name}, args...)
		// A command we don't expect to actually run (and it won't, because we won't call .Run()).
		return exec.CommandContext(ctx, "true")
	}
	defer func() { execCommandContext = orig }()

	// Call Init; it will attempt to run "true". That succeeds and allows us to inspect args.
	runner, err := NewTofuRunner(
		ctx,
		&Config{workDir: "/tmp", tofuBinaryPath: "/bin/tofu"},
		io.Discard,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("NewTofuRunner() returned error: %v", err)
	}

	if err := runner.Init(ctx, lockfileMode); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", wantArgs, gotArgs)
	}
}
