// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"context"
	"io"
	"os/exec"
)

type LocalCommandRunner interface {
	Run(
		ctx context.Context,
		command []string,
		workingDir string,
		stdout, stderr io.Writer,
	) error
}

type LocalCommandRunnerImpl struct{}

func (*LocalCommandRunnerImpl) Run(
	ctx context.Context,
	command []string,
	workingDir string,
	stdout, stderr io.Writer,
) error {
	commandProc := exec.CommandContext(ctx, command[0], command[1:]...)

	commandProc.Stdout = stdout
	commandProc.Stderr = stderr

	commandProc.Dir = workingDir

	return commandProc.Run()
}
