// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows

package util

import (
	"os"
	"os/exec"
)

// Console control events already reach every process attached to the console.
func prepareInterruptible(_ *exec.Cmd) {}

func interruptProcess(_ *os.Process) {}
