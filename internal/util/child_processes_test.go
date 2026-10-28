// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exits cleanly only when interrupted, after a short graceful stop. Its pid is
// written to "$1" once the interrupt handling is set up.
const stopsOnInterrupt = `trap 'sleep 0.3; exit 0' INT
echo $$ > "$1.tmp"; mv "$1.tmp" "$1"
while :; do sleep 0.1; done`

// returningTracker lets interrupted commands return, so tests can observe them.
func returningTracker() *childProcessTracker {
	tracker := newChildProcessTracker()
	tracker.afterInterruptedCommand = func() {}

	return tracker
}

func stopsOnInterruptCommand(t *testing.T) (*exec.Cmd, string) {
	t.Helper()

	ready := filepath.Join(t.TempDir(), "ready")

	return exec.CommandContext(t.Context(), "sh", "-c", stopsOnInterrupt, "sh", ready), ready
}

func runInBackground(tracker *childProcessTracker, cmd *exec.Cmd) <-chan error {
	result := make(chan error, 1)
	go func() { result <- tracker.run(cmd) }()

	return result
}

func waitUntilReady(t *testing.T, ready string) int {
	t.Helper()

	require.Eventually(t, func() bool {
		_, err := os.Stat(ready)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)

	content, err := os.ReadFile(ready)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	require.NoError(t, err)

	return pid
}

func TestWaitForChildProcessesReturnsWhenNoneRun(t *testing.T) {
	t.Parallel()

	// Given: no running command.
	tracker := returningTracker()

	// When: waiting for commands.
	returned := make(chan struct{})
	go func() {
		tracker.wait()
		close(returned)
	}()

	// Then: the wait returns immediately.
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return without running commands")
	}
}

func TestInterruptStopsCommandGracefullyAndWaitOutlastsIt(t *testing.T) {
	t.Parallel()

	// Given: a running command that needs time to stop after an interrupt.
	tracker := returningTracker()
	cmd, ready := stopsOnInterruptCommand(t)
	result := runInBackground(tracker, cmd)
	pid := waitUntilReady(t, ready)

	// Then: it runs in its own process group, out of reach of terminal interrupts.
	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)
	require.Equal(t, pid, pgid)

	// When: interrupting and waiting.
	started := time.Now()
	tracker.interrupt()
	tracker.wait()

	// Then: the command stopped through its interrupt handling, and the wait
	// lasted until it exited.
	require.NoError(t, <-result)
	require.GreaterOrEqual(t, time.Since(started), 300*time.Millisecond)
}

func TestInterruptedCommandDoesNotResumeItsCaller(t *testing.T) {
	t.Parallel()

	// Given: a running command whose caller resumes only after it returns.
	tracker := newChildProcessTracker()
	cmd, ready := stopsOnInterruptCommand(t)
	result := runInBackground(tracker, cmd)
	waitUntilReady(t, ready)

	// When: the command is interrupted and has stopped.
	tracker.interrupt()
	tracker.wait()

	// Then: its caller does not resume, so it cannot record the stop as a failure.
	select {
	case <-result:
		t.Fatal("caller of an interrupted command resumed")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestCommandStartedAfterInterruptIsInterrupted(t *testing.T) {
	t.Parallel()

	// Given: an interrupt has already been requested.
	tracker := returningTracker()
	tracker.interrupt()

	// When: a command starts afterwards.
	cmd, _ := stopsOnInterruptCommand(t)
	result := runInBackground(tracker, cmd)

	// Then: it is interrupted as well and stops, possibly before its interrupt
	// handling is set up.
	select {
	case <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("command started after the interrupt kept running")
	}
}
