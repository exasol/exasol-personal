// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package tofu

import (
	"context"
	"io"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/util"
)

// nolint: paralleltest
func TestTofuRunnerInit_UsesReadonlyLockfileByDefault(t *testing.T) {
	testTofuRunnerInitArgs(t, LockfileReadonly, []string{"/bin/tofu", "init", "-lockfile=readonly"})
}

// nolint: paralleltest
func TestTofuRunnerInit_UsesUpdateLockfileWhenEnabled(t *testing.T) {
	testTofuRunnerInitArgs(t, LockfileUpdate, []string{"/bin/tofu", "init", "-lockfile=update"})
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

// nolint: paralleltest
func TestTofuRunnerIsWaitedForUntilItExits(t *testing.T) {
	// Given: a tofu command that takes a while to stop.
	ctx := context.Background()

	orig := execCommandContext
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "0.5")
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

	started := time.Now()
	go func() { _ = runner.Init(ctx, LockfileReadonly) }()
	time.Sleep(100 * time.Millisecond)

	// When: waiting for child processes, as on an interrupt.
	util.WaitForChildProcesses()

	// Then: the wait returns only after tofu exited.
	if elapsed := time.Since(started); elapsed < 450*time.Millisecond {
		t.Fatalf("wait returned after %v, while tofu was still running", elapsed)
	}
}
