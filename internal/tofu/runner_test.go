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
	ctx := context.Background()

	var gotArgs []string
	orig := execCommandContext
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{name}, args...)
		// A command we don't expect to actually run (and it won't, because we won't call .Run()).
		return exec.CommandContext(ctx, "true")
	}
	defer func() { execCommandContext = orig }()

	r := NewTofuRunner(Config{workDir: "/tmp", tofuBinaryPath: "/bin/tofu"}, io.Discard, io.Discard)

	// Call Init; it will attempt to run "true". That succeeds and allows us to inspect args.
	if err := r.Init(ctx, LockfileReadonly); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	want := []string{"/bin/tofu", "init", "-lockfile=readonly"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, gotArgs)
	}
}

// nolint: paralleltest
func TestTofuRunnerInit_UsesUpdateLockfileWhenEnabled(t *testing.T) {
	ctx := context.Background()

	var gotArgs []string
	orig := execCommandContext
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{name}, args...)
		return exec.CommandContext(ctx, "true")
	}
	defer func() { execCommandContext = orig }()

	r := NewTofuRunner(Config{workDir: "/tmp", tofuBinaryPath: "/bin/tofu"}, io.Discard, io.Discard)

	if err := r.Init(ctx, LockfileUpdate); err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	want := []string{"/bin/tofu", "init", "-lockfile=update"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("unexpected args.\nwant: %v\ngot:  %v", want, gotArgs)
	}
}
