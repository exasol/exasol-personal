// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package util

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

// A terminal interrupt must not reach the command directly: OpenTofu exits
// without saving its state on a second interrupt, so it gets exactly one, from
// InterruptChildProcesses.
func prepareInterruptible(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func interruptProcess(process *os.Process) {
	if err := process.Signal(os.Interrupt); err != nil {
		slog.Debug("could not interrupt command", "pid", process.Pid, "error", err.Error())
	}
}
