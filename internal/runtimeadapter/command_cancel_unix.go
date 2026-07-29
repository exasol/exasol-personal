// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package runtimeadapter

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureCommandCancellation(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := command.Process.Signal(syscall.SIGTERM)
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}

		return err
	}
	command.WaitDelay = commandTerminationGrace
}
