// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/localinstall"
)

const vmSharedDir = "/mnt/host"

const runnerCommandPrefixArgumentCount = 2

type runnerExecutionEnvironment struct {
	runnerPath string
	workDir    string
}

func newRunnerExecutionEnvironment(runnerPath, workDir string) *runnerExecutionEnvironment {
	return &runnerExecutionEnvironment{runnerPath: runnerPath, workDir: workDir}
}

func (environment *runnerExecutionEnvironment) Sync(
	ctx context.Context,
	stdout, stderr io.Writer,
) error {
	return environment.Run(ctx, nil, stdout, stderr, "sync")
}

func (environment *runnerExecutionEnvironment) Run(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
	command ...string,
) error {
	if len(command) == 0 {
		return errors.New("execution environment command is empty")
	}
	args := make(
		[]string, 0, len(command)+runnerCommandPrefixArgumentCount,
	)
	args = append(args, "run", "--")
	args = append(args, command...)
	cmd := exec.CommandContext(ctx, environment.runnerPath, args...)
	cmd.Dir = environment.workDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

func (environment *runnerExecutionEnvironment) PathExists(
	ctx context.Context,
	runtimePath string,
) (bool, error) {
	err := environment.Run(
		ctx, nil, nil, nil,
		"sh", "-c", `[ -e "$1" ] || [ -L "$1" ]`, "sh", runtimePath,
	)
	if err == nil {
		return true, nil
	}
	if commandExitedWith(err, 1) {
		return false, nil
	}

	return false, fmt.Errorf("failed to inspect runtime path %s: %w", runtimePath, err)
}

func (environment *runnerExecutionEnvironment) DirectoryHasEntries(
	ctx context.Context,
	directory string,
) (bool, error) {
	exists, err := environment.PathExists(ctx, directory)
	if err != nil || !exists {
		return false, err
	}

	var output bytes.Buffer
	if err := environment.Run(
		ctx, nil, &output, nil,
		"find", directory, "-mindepth", "1", "-maxdepth", "1", "-print", "-quit",
	); err != nil {
		return false, fmt.Errorf("failed to inspect runtime directory %s: %w", directory, err)
	}

	return strings.TrimSpace(output.String()) != "", nil
}

func (environment *runnerExecutionEnvironment) MkdirAll(
	ctx context.Context,
	directory string,
	mode os.FileMode,
) error {
	return environment.Run(
		ctx, nil, nil, nil,
		"sh", "-c", `umask 0; mkdir -p -- "$2"; chmod "$1" "$2"`,
		"sh", formatFileMode(mode), directory,
	)
}

func (environment *runnerExecutionEnvironment) MkdirTemp(
	ctx context.Context,
	parent, pattern string,
) (string, error) {
	template := runtimeTempTemplate(pattern)
	var output bytes.Buffer
	if err := environment.Run(
		ctx, nil, &output, nil, "mktemp", "-d", path.Join(parent, template),
	); err != nil {
		return "", fmt.Errorf("failed to create runtime temporary directory: %w", err)
	}
	result := strings.TrimSpace(output.String())
	if result == "" {
		return "", errors.New("runtime mktemp returned an empty path")
	}

	return result, nil
}

func (environment *runnerExecutionEnvironment) RemoveFile(
	ctx context.Context,
	runtimePath string,
) error {
	return environment.Run(ctx, nil, nil, nil, "rm", "-f", "--", runtimePath)
}

func (environment *runnerExecutionEnvironment) RemoveDir(
	ctx context.Context,
	runtimePath string,
) error {
	return environment.Run(
		ctx, nil, nil, nil,
		"sh", "-c", `[ ! -e "$1" ] || rmdir -- "$1"`, "sh", runtimePath,
	)
}

func (environment *runnerExecutionEnvironment) RemoveAll(
	ctx context.Context,
	runtimePath string,
) error {
	return environment.Run(ctx, nil, nil, nil, "rm", "-rf", "--", runtimePath)
}

func (environment *runnerExecutionEnvironment) Rename(
	ctx context.Context,
	oldPath, newPath string,
) error {
	return environment.Run(ctx, nil, nil, nil, "mv", "--", oldPath, newPath)
}

func (environment *runnerExecutionEnvironment) WriteFileAtomically(
	ctx context.Context,
	filePath string,
	data []byte,
	dirMode, fileMode os.FileMode,
) error {
	const script = `set -eu
target=$1
directory=${target%/*}
[ "$directory" != "$target" ] || directory=.
umask 0
if [ ! -d "$directory" ]; then
    mkdir -p -- "$directory"
    chmod "$2" "$directory"
fi
temporary=$(mktemp "$directory/.runtime-write-XXXXXX")
trap 'rm -f -- "$temporary"' EXIT
cat > "$temporary"
chmod "$3" "$temporary"
mv -- "$temporary" "$target"
trap - EXIT`

	if err := environment.Run(
		ctx,
		bytes.NewReader(data),
		nil,
		nil,
		"sh",
		"-c",
		script,
		"sh",
		filePath,
		formatFileMode(dirMode),
		formatFileMode(fileMode),
	); err != nil {
		return fmt.Errorf("failed to replace runtime file %s: %w", filePath, err)
	}

	return nil
}

func commandExitedWith(err error, exitCode int) bool {
	var exitError *exec.ExitError

	return errors.As(err, &exitError) && exitError.ExitCode() == exitCode
}

func formatFileMode(mode os.FileMode) string {
	return strconv.FormatUint(uint64(mode.Perm()), 8)
}

func runtimeTempTemplate(pattern string) string {
	if wildcard := strings.LastIndex(pattern, "*"); wildcard >= 0 {
		return pattern[:wildcard] + "XXXXXX" + pattern[wildcard+1:]
	}

	return pattern + "XXXXXX"
}

type vmPathMapper struct {
	hostRoot    string
	runtimeRoot string
}

func newVMPathMapper(hostRoot string) vmPathMapper {
	return vmPathMapper{hostRoot: filepath.Clean(hostRoot), runtimeRoot: vmSharedDir}
}

func (mapper vmPathMapper) Map(hostPath string) (localinstall.RuntimePath, error) {
	hostPath = filepath.Clean(hostPath)
	relative, err := filepath.Rel(mapper.hostRoot, hostPath)
	if err != nil {
		return localinstall.RuntimePath{}, fmt.Errorf(
			"failed to map runtime path %s: %w", hostPath, err,
		)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return localinstall.RuntimePath{}, fmt.Errorf(
			"host path %s is outside the VM shared directory %s",
			hostPath,
			mapper.hostRoot,
		)
	}

	return localinstall.RuntimePath{
		HostPath:    hostPath,
		RuntimePath: path.Join(mapper.runtimeRoot, filepath.ToSlash(relative)),
	}, nil
}
