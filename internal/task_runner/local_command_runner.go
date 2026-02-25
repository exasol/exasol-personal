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
	command_proc := exec.CommandContext(ctx, command[0], command[1:]...)

	command_proc.Stdout = stdout
	command_proc.Stderr = stderr

	command_proc.Dir = workingDir

	return command_proc.Run()
}
