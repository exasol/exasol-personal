// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"time"
)

type CommandRunner interface {
	Run(
		ctx context.Context,
		stdin io.Reader,
		stdout, stderr io.Writer,
		name string,
		args ...string,
	) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath(name string) (string, error)
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
	name string,
	args ...string,
) error {
	command := exec.CommandContext(ctx, name, args...)
	configureCommandCancellation(command)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr

	return command.Run()
}

func (OSCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	configureCommandCancellation(command)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, &CommandError{
			Name:   name,
			Args:   append([]string(nil), args...),
			Stderr: stderr.String(),
			Err:    err,
		}
	}

	return stdout.Bytes(), nil
}

const commandTerminationGrace = 10 * time.Second

func (OSCommandRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type CommandError struct {
	Name   string
	Args   []string
	Stderr string
	Err    error
}

func (err *CommandError) Error() string {
	if err.Stderr == "" {
		return err.Name + ": " + err.Err.Error()
	}

	return err.Name + ": " + err.Err.Error() + ": " + err.Stderr
}

func (err *CommandError) Unwrap() error {
	return err.Err
}
