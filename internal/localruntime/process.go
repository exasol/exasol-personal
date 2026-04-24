// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrRuntimeNotRunning = errors.New("local runtime is not running")

const processPollInterval = 100 * time.Millisecond
const localProbeTimeout = 100 * time.Millisecond

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

	return os.WriteFile(
		r.layout.PIDFilePath(),
		[]byte(strconv.Itoa(pid)+"\n"),
		localRuntimeFileMode,
	)
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

func (r *Runtime) RuntimeActive() (bool, int, error) {
	running, pid, err := r.RunnerRunning()
	if err != nil {
		return false, 0, err
	}
	if running {
		return true, pid, nil
	}

	socketActive, err := r.controlSocketActive()
	if err != nil {
		return false, 0, err
	}
	if socketActive {
		return true, 0, nil
	}

	portsActive, err := r.connectionPortsActive()
	if err != nil {
		return false, 0, err
	}
	if portsActive {
		return true, 0, nil
	}

	return false, 0, nil
}

func (*Runtime) WaitForRunnerExit(ctx context.Context, pid int) error {
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

func (r *Runtime) WaitForInactive(ctx context.Context) error {
	ticker := time.NewTicker(processPollInterval)
	defer ticker.Stop()

	for {
		active, _, err := r.RuntimeActive()
		if err != nil {
			return err
		}
		if !active {
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

func (r *Runtime) controlSocketActive() (bool, error) {
	socketPath := r.layout.ControlSocketPath()
	ok, err := hasSocket(socketPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
	if !ok {
		return false, nil
	}

	conn, err := (&net.Dialer{Timeout: localProbeTimeout}).Dial("unix", socketPath)
	if err != nil {
		return false, nil
	}
	_ = conn.Close()

	return true, nil
}

func (r *Runtime) connectionPortsActive() (bool, error) {
	state, err := r.LoadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	for _, portName := range []string{dbPortName, uiPortName} {
		port := state.Ports[portName]
		if port <= 0 {
			continue
		}
		if loopbackPortActive(port) {
			return true, nil
		}
	}

	return false, nil
}

func loopbackPortActive(port int) bool {
	conn, err := (&net.Dialer{Timeout: localProbeTimeout}).Dial(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
	)
	if err != nil {
		return false
	}
	_ = conn.Close()

	return true
}
