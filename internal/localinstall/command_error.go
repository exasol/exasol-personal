// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

// Bounds memory for chatty commands; a runtime prints its error last.
const stderrTailLimit = 64 * 1024

// CommandError carries a failed container runtime command's stderr, because
// the exit status alone does not say why the command failed.
type CommandError struct {
	Stderr string
	Err    error
}

func (failure *CommandError) Error() string {
	if cause := sidecar.ReportedCause(failure.Stderr); cause != "" {
		return failure.Err.Error() + ": " + cause
	}

	return failure.Err.Error()
}

func (failure *CommandError) Unwrap() error {
	return failure.Err
}

func CommandStderr(err error) string {
	if commandErr, ok := errors.AsType[*CommandError](err); ok {
		return commandErr.Stderr
	}

	return ""
}

func newCommandError(err error, stderr *stderrTail) error {
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(string(stderr.data))
	if output == "" {
		return err
	}

	return &CommandError{Stderr: output, Err: err}
}

type stderrTail struct {
	data []byte
}

func (tail *stderrTail) Write(chunk []byte) (int, error) {
	tail.data = append(tail.data, chunk...)
	if excess := len(tail.data) - stderrTailLimit; excess > 0 {
		tail.data = tail.data[excess:]
	}

	return len(chunk), nil
}

type commandStreams struct {
	stdout io.Writer
	stderr io.Writer
}

// newCommandStreams adds the stderr tail while keeping exec's guarantee that a
// writer shared by stdout and stderr is never written concurrently.
func newCommandStreams(out, outErr io.Writer, tail *stderrTail) commandStreams {
	stderr := io.Writer(tail)
	if outErr != nil {
		stderr = io.MultiWriter(outErr, tail)
	}
	if out == nil {
		return commandStreams{stderr: stderr}
	}
	lock := &sync.Mutex{}

	return commandStreams{
		stdout: lockedWriter{lock: lock, writer: out},
		stderr: lockedWriter{lock: lock, writer: stderr},
	}
}

type lockedWriter struct {
	lock   *sync.Mutex
	writer io.Writer
}

func (locked lockedWriter) Write(chunk []byte) (int, error) {
	locked.lock.Lock()
	defer locked.lock.Unlock()

	return locked.writer.Write(chunk)
}
