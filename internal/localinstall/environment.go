// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// RuntimePath identifies the same artifact from the host and execution environment.
type RuntimePath struct {
	HostPath    string
	RuntimePath string
}

func IdentityRuntimePath(path string) RuntimePath {
	return RuntimePath{HostPath: path, RuntimePath: path}
}

// ExecutionEnvironment isolates installation logic from its command transport and filesystem.
// It intentionally owns both command and filesystem operations for the same runtime.
// nolint: interfacebloat
type ExecutionEnvironment interface {
	Sync(ctx context.Context, stdout, stderr io.Writer) error
	Run(
		ctx context.Context,
		stdin io.Reader,
		stdout, stderr io.Writer,
		command ...string,
	) error
	PathExists(ctx context.Context, path string) (bool, error)
	DirectoryHasEntries(ctx context.Context, path string) (bool, error)
	MkdirAll(ctx context.Context, path string, mode os.FileMode) error
	MkdirTemp(ctx context.Context, parent, pattern string) (string, error)
	RemoveFile(ctx context.Context, path string) error
	RemoveDir(ctx context.Context, path string) error
	RemoveAll(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	WriteFileAtomically(
		ctx context.Context,
		path string,
		data []byte,
		dirMode, fileMode os.FileMode,
	) error
}

type DirectExecutionEnvironment struct {
	commandPrefix []string
}

func NewDirectExecutionEnvironment(commandPrefix []string) *DirectExecutionEnvironment {
	return &DirectExecutionEnvironment{commandPrefix: append([]string(nil), commandPrefix...)}
}

func (environment *DirectExecutionEnvironment) Sync(
	ctx context.Context,
	stdout, stderr io.Writer,
) error {
	return environment.Run(ctx, nil, stdout, stderr, "sync")
}

func (environment *DirectExecutionEnvironment) Run(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
	command ...string,
) error {
	cmdLine := make([]string, 0, len(environment.commandPrefix)+len(command))
	cmdLine = append(cmdLine, environment.commandPrefix...)
	cmdLine = append(cmdLine, command...)
	if len(cmdLine) == 0 {
		return errors.New("execution environment command is empty")
	}
	cmd := exec.CommandContext(ctx, cmdLine[0], cmdLine[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

func (*DirectExecutionEnvironment) PathExists(_ context.Context, path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, err
}

func (*DirectExecutionEnvironment) DirectoryHasEntries(
	_ context.Context,
	path string,
) (bool, error) {
	directory, err := os.Open(path) //nolint:gosec // runtime-owned path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
	defer func() { _ = directory.Close() }()

	_, err = directory.Readdirnames(1)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}

	return false, err
}

func (*DirectExecutionEnvironment) MkdirAll(
	_ context.Context,
	path string,
	mode os.FileMode,
) error {
	return os.MkdirAll(path, mode)
}

func (*DirectExecutionEnvironment) MkdirTemp(
	_ context.Context,
	parent, pattern string,
) (string, error) {
	return os.MkdirTemp(parent, pattern)
}

func (*DirectExecutionEnvironment) RemoveFile(_ context.Context, path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func (*DirectExecutionEnvironment) RemoveDir(_ context.Context, path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func (*DirectExecutionEnvironment) RemoveAll(_ context.Context, path string) error {
	return os.RemoveAll(path)
}

func (*DirectExecutionEnvironment) Rename(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (*DirectExecutionEnvironment) WriteFileAtomically(
	_ context.Context,
	path string,
	data []byte,
	dirMode, fileMode os.FileMode,
) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, dirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".runtime-write-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}

	return nil
}
