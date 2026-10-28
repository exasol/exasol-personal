// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

type childProcessTracker struct {
	mtx         sync.Mutex
	idle        *sync.Cond
	running     map[*os.Process]struct{}
	interrupted bool
	// The interrupt handler exits the process once the commands stopped; their
	// callers must not resume meanwhile and record the stop as a failure.
	afterInterruptedCommand func()
}

var childProcesses = newChildProcessTracker()

func newChildProcessTracker() *childProcessTracker {
	tracker := &childProcessTracker{
		running:                 make(map[*os.Process]struct{}),
		afterInterruptedCommand: func() { select {} },
	}
	tracker.idle = sync.NewCond(&tracker.mtx)

	return tracker
}

// RunInterruptible runs cmd like cmd.Run, but lets InterruptChildProcesses
// stop it gracefully and WaitForChildProcesses wait for it. It does not return
// once InterruptChildProcesses was called.
func RunInterruptible(cmd *exec.Cmd) error {
	return childProcesses.run(cmd)
}

// InterruptChildProcesses asks every running and later started command of
// RunInterruptible to stop, once.
func InterruptChildProcesses() {
	childProcesses.interrupt()
}

// WaitForChildProcesses blocks until no command of RunInterruptible is running.
func WaitForChildProcesses() {
	childProcesses.wait()
}

func (t *childProcessTracker) run(cmd *exec.Cmd) error {
	prepareInterruptible(cmd)

	t.mtx.Lock()
	if err := cmd.Start(); err != nil {
		interrupted := t.interrupted
		t.mtx.Unlock()
		if interrupted {
			t.afterInterruptedCommand()
		}

		return err
	}
	process := cmd.Process
	t.running[process] = struct{}{}
	if t.interrupted {
		interruptProcess(process)
	}
	t.mtx.Unlock()

	err := cmd.Wait()

	t.mtx.Lock()
	delete(t.running, process)
	if len(t.running) == 0 {
		t.idle.Broadcast()
	}
	interrupted := t.interrupted
	t.mtx.Unlock()

	if interrupted {
		t.afterInterruptedCommand()
	}

	return err
}

func (t *childProcessTracker) interrupt() {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if t.interrupted {
		return
	}
	t.interrupted = true

	for process := range t.running {
		interruptProcess(process)
	}
}

func (t *childProcessTracker) wait() {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if len(t.running) > 0 {
		slog.Warn("Waiting for running commands to stop", "count", len(t.running))
	}

	for len(t.running) > 0 {
		t.idle.Wait()
	}
}
