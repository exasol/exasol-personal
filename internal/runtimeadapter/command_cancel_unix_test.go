// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package runtimeadapter

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestOSCommandRunnerCancelsChildWithTermSignal(t *testing.T) {
	t.Parallel()

	// Given
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// When
	err := (OSCommandRunner{}).Run(
		ctx,
		nil,
		io.Discard,
		io.Discard,
		"sh",
		"-c",
		"trap 'exit 42' TERM; while :; do sleep 1; done",
	)

	// Then
	if err == nil {
		t.Fatal("expected cancelled command to fail")
	}
	if !errors.Is(err, context.Canceled) {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
			t.Fatalf("expected context cancellation or TERM trap exit, got %v", err)
		}
	}
}
