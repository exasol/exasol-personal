// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrRuntimeNotRunning = errors.New("local runtime is not running")

const processPollInterval = 100 * time.Millisecond

func (r *Runtime) ReadRunnerPID() (int, error) {
	data, err := os.ReadFile(r.layout.PIDFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, ErrRuntimeNotRunning
		}

		return 0, fmt.Errorf("failed to read local runtime PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("failed to parse local runtime PID: %w", err)
	}

	return pid, nil
}

func (r *Runtime) WriteRunnerPID(pid int) error {
	if pid <= 0 {
		return errors.New("local runtime PID must be positive")
	}
	if err := r.EnsureRoot(); err != nil {
		return err
	}

	return os.WriteFile(r.layout.PIDFilePath(), []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func (r *Runtime) RunnerRunning() (bool, int, error) {
	pid, err := r.ReadRunnerPID()
	if err != nil {
		if errors.Is(err, ErrRuntimeNotRunning) {
			return false, 0, nil
		}

		return false, 0, err
	}

	return IsProcessRunning(pid), pid, nil
}

func (r *Runtime) WaitForRunnerExit(ctx context.Context, pid int) error {
	ticker := time.NewTicker(processPollInterval)
	defer ticker.Stop()

	for {
		if !IsProcessRunning(pid) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runtime) ResetControlState() error {
	for _, path := range []string{
		r.layout.ControlSocketPath(),
		r.layout.RuntimeStatePath(),
		r.layout.StopRequestPath(),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local runtime control artifact %q: %w", path, err)
		}
	}

	return nil
}

func (r *Runtime) CleanupTransientState() error {
	for _, path := range []string{
		r.layout.ControlSocketPath(),
		r.layout.StopRequestPath(),
		r.layout.PIDFilePath(),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local runtime transient artifact %q: %w", path, err)
		}
	}

	return nil
}
