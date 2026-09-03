// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVMPathMapperMapsOnlySharedPaths(t *testing.T) {
	t.Parallel()

	hostRoot := filepath.Join(t.TempDir(), "vm share")
	mapper := newVMPathMapper(hostRoot)
	mapped, err := mapper.Map(filepath.Join(hostRoot, "resources", "nano image.tar"))
	if err != nil {
		t.Fatalf("expected shared path to map: %v", err)
	}
	if mapped.HostPath != filepath.Join(hostRoot, "resources", "nano image.tar") {
		t.Fatalf("unexpected host path: %#v", mapped)
	}
	if mapped.RuntimePath != "/mnt/host/resources/nano image.tar" {
		t.Fatalf("unexpected runtime path: %#v", mapped)
	}

	_, err = mapper.Map(filepath.Join(filepath.Dir(hostRoot), "outside.tar"))
	if err == nil || !strings.Contains(err.Error(), "outside the VM shared directory") {
		t.Fatalf("expected outside path to be rejected, got %v", err)
	}
}

func TestRunnerExecutionEnvironmentPreservesCommandIOAndExitStatus(t *testing.T) {
	t.Parallel()
	requireRunnerEnvironmentPOSIX(t)

	environment := newTestRunnerExecutionEnvironment(t)
	var stdout, stderr bytes.Buffer
	err := environment.Run(
		context.Background(),
		strings.NewReader("input with spaces"),
		&stdout,
		&stderr,
		"sh",
		"-c",
		`read value; printf '<%s>' "$value"; printf 'problem' >&2; exit 23`,
	)
	if !commandExitedWith(err, 23) {
		t.Fatalf("expected exit status 23, got %v", err)
	}
	if stdout.String() != "<input with spaces>" || stderr.String() != "problem" {
		t.Fatalf("unexpected command output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunnerExecutionEnvironmentSyncsInsideRuntime(t *testing.T) {
	t.Parallel()
	requireRunnerEnvironmentPOSIX(t)

	environment := newTestRunnerExecutionEnvironment(t)
	if err := environment.Sync(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected runtime sync to succeed, got %v", err)
	}
}

func TestRunnerExecutionEnvironmentFilesystemOperations(t *testing.T) {
	t.Parallel()
	requireRunnerEnvironmentPOSIX(t)

	ctx := context.Background()
	environment := newTestRunnerExecutionEnvironment(t)
	root := filepath.Join(t.TempDir(), "runtime root")
	directory := filepath.Join(root, "directory")
	if err := environment.MkdirAll(ctx, directory, 0o750); err != nil {
		t.Fatalf("mkdir all failed: %v", err)
	}
	if exists, err := environment.PathExists(ctx, directory); err != nil || !exists {
		t.Fatalf("expected directory to exist, exists=%t err=%v", exists, err)
	}
	if populated, err := environment.DirectoryHasEntries(ctx, directory); err != nil || populated {
		t.Fatalf("expected empty directory, populated=%t err=%v", populated, err)
	}
	// Directory access requires the owner execute bit.
	if err := os.Chmod(directory, 0o700); err != nil { //nolint:gosec
		t.Fatalf("failed to set existing directory mode: %v", err)
	}

	filePath := filepath.Join(directory, "state file")
	if err := environment.WriteFileAtomically(
		ctx, filePath, []byte("state\n"), 0o750, 0o640,
	); err != nil {
		t.Fatalf("atomic write failed: %v", err)
	}
	if populated, err := environment.DirectoryHasEntries(ctx, directory); err != nil || !populated {
		t.Fatalf("expected populated directory, populated=%t err=%v", populated, err)
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("failed to inspect existing directory mode: %v", err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("existing directory mode changed to %v", directoryInfo.Mode().Perm())
	}
	content, err := os.ReadFile(filePath)
	if err != nil || string(content) != "state\n" {
		t.Fatalf("unexpected file content %q, err=%v", string(content), err)
	}

	temporary, err := environment.MkdirTemp(ctx, root, ".migration-*")
	if err != nil {
		t.Fatalf("mkdir temp failed: %v", err)
	}
	renamed := temporary + "-renamed"
	if err := environment.Rename(ctx, temporary, renamed); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if err := environment.RemoveDir(ctx, renamed); err != nil {
		t.Fatalf("remove dir failed: %v", err)
	}
	if err := environment.RemoveFile(ctx, filePath); err != nil {
		t.Fatalf("remove file failed: %v", err)
	}
	if err := environment.RemoveAll(ctx, root); err != nil {
		t.Fatalf("remove all failed: %v", err)
	}
	assertRunnerPathAbsent(ctx, t, environment, root)
}

func assertRunnerPathAbsent(
	ctx context.Context,
	t *testing.T,
	environment *runnerExecutionEnvironment,
	path string,
) {
	t.Helper()

	exists, err := environment.PathExists(ctx, path)
	if err != nil {
		t.Fatalf("path existence check failed: %v", err)
	}
	if exists {
		t.Fatalf("expected %q to be absent", path)
	}
}

func TestRuntimeTempTemplateMatchesMkdirTempSemantics(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"prefix-*":        "prefix-XXXXXX",
		"prefix-*-suffix": "prefix-XXXXXX-suffix",
		"prefix":          "prefixXXXXXX",
	}
	for pattern, expected := range tests {
		if actual := runtimeTempTemplate(pattern); actual != expected {
			t.Fatalf("runtimeTempTemplate(%q) = %q, want %q", pattern, actual, expected)
		}
	}
}

func TestCommandExitedWith(t *testing.T) {
	t.Parallel()

	command := exec.CommandContext(context.Background(), "sh", "-c", "exit 7")
	err := command.Run()
	if !commandExitedWith(err, 7) {
		t.Fatalf("expected exit 7, got %v", err)
	}
	if commandExitedWith(errors.New("not an exit error"), 7) {
		t.Fatal("expected ordinary error not to match")
	}
}

func newTestRunnerExecutionEnvironment(t *testing.T) *runnerExecutionEnvironment {
	t.Helper()

	workDir := t.TempDir()
	runnerPath := filepath.Join(t.TempDir(), "launcher")
	script := `#!/bin/sh
set -eu
[ "$1" = run ]
shift
[ "$1" = -- ]
shift
exec "$@"
`
	const executableMode = 0o700
	if err := os.WriteFile(runnerPath, []byte(script), executableMode); err != nil {
		t.Fatalf("failed to write fake runner: %v", err)
	}

	return newRunnerExecutionEnvironment(runnerPath, workDir)
}

func requireRunnerEnvironmentPOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake runner requires a POSIX shell")
	}
}
