// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/exasol/exasol-personal/internal/util"
)

func assertOnlySecretsFile(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	expectNoErr(t, err)
	if len(entries) != 1 || entries[0].Name() != secretsFileName {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected only %s in %s, found %v", secretsFileName, dir, names)
	}
}

func TestWriteSecretsRoundTrips(t *testing.T) {
	t.Parallel()

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	expected := &Secrets{DbPassword: "first", AdminUiPassword: "admin"}

	// When
	expectNoErr(t, WriteSecrets(deployment.Root(), expected))
	actual, err := ReadSecrets(deployment)

	// Then
	expectNoErr(t, err)
	if *actual != *expected {
		t.Fatalf("expected %+v, got %+v", *expected, *actual)
	}
	assertOnlySecretsFile(t, deployment.Root())
}

func TestWriteSecretsReplacesFileWithOwnerOnlyMode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not apply POSIX file modes")
	}

	// Given
	deployment := NewDeploymentDir(t.TempDir())
	//nolint:gosec // a readable previous file proves the mode is reset
	expectNoErr(t, os.WriteFile(deployment.SecretsPath(), []byte(`{}`), 0o644))

	// When
	expectNoErr(t, WriteSecrets(deployment.Root(), &Secrets{DbPassword: "second"}))

	// Then
	info, err := os.Stat(deployment.SecretsPath())
	expectNoErr(t, err)
	if mode := info.Mode().Perm(); mode != secretsFileMode {
		t.Fatalf("expected mode %o, got %o", secretsFileMode, mode)
	}
}

func TestWriteSecretsFailureKeepsPreviousFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not apply POSIX directory permissions")
	}

	// Given
	dir := t.TempDir()
	deployment := NewDeploymentDir(dir)
	previous := &Secrets{DbPassword: "previous"}
	expectNoErr(t, WriteSecrets(dir, previous))
	//nolint:gosec // remove write bit to force the replacement to fail
	mustChmod(t, dir, 0o500)
	//nolint:gosec
	defer os.Chmod(dir, 0o700)

	// When
	err := WriteSecrets(dir, &Secrets{DbPassword: "replacement"})

	// Then
	expectErr(t, err)
	//nolint:gosec
	mustChmod(t, dir, 0o700)
	actual, readErr := ReadSecrets(deployment)
	expectNoErr(t, readErr)
	if *actual != *previous {
		t.Fatalf("expected previous secrets %+v, got %+v", *previous, *actual)
	}
	assertOnlySecretsFile(t, dir)
}

func TestWriteSecretsRemovesStaleTemporaryFiles(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	stale := filepath.Join(dir, "."+secretsFileName+".tmp-123")
	expectNoErr(t, os.WriteFile(stale, []byte(`{"dbPassword":"stale"}`), secretsFileMode))

	// When
	err := WriteSecrets(dir, &Secrets{DbPassword: "current"})

	// Then
	expectNoErr(t, err)
	assertOnlySecretsFile(t, dir)
}

//nolint:paralleltest // Replaces the process-wide signal handler and rename hook.
func TestWriteSecretsInterruptRemovesTemporaryFile(t *testing.T) {
	// Given
	dir := t.TempDir()
	deployment := NewDeploymentDir(dir)
	previous := &Secrets{DbPassword: "previous"}
	expectNoErr(t, WriteSecrets(dir, previous))

	util.StopSignalHandler()
	t.Cleanup(util.StopSignalHandler)
	signals := make(chan os.Signal, 1)
	exited := make(chan struct{})
	util.StartSignalHandlerWithChannel(signals, func(os.Signal) { close(exited) })
	originalHook := beforeAtomicRename
	t.Cleanup(func() { beforeAtomicRename = originalHook })
	var temporariesAtExit []string
	beforeAtomicRename = func() {
		signals <- syscall.SIGINT
		<-exited
		// The real launcher exits here, so nothing after this point cleans up.
		temporariesAtExit, _ = temporaryFiles(dir, "."+secretsFileName+".tmp-")
	}

	// When
	_ = WriteSecrets(dir, &Secrets{DbPassword: "interrupted"})

	// Then
	if len(temporariesAtExit) != 0 {
		t.Fatalf("expected no temporary secrets when the launcher exits, got %v", temporariesAtExit)
	}
	actual, err := ReadSecrets(deployment)
	expectNoErr(t, err)
	if *actual != *previous {
		t.Fatalf("expected previous secrets %+v, got %+v", *previous, *actual)
	}
	assertOnlySecretsFile(t, dir)
}

func TestWriteSecretsTreatsDirectoryNamesLiterally(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		directory string
		sibling   string
	}{
		{name: "bracket expression", directory: "deployment[1]", sibling: "deployment1"},
		{name: "unmatched bracket", directory: "deployment[", sibling: "deployment"},
		{name: "wildcards", directory: "deploy*?", sibling: "deployXY"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if runtime.GOOS == "windows" && strings.ContainsAny(test.directory, "*?") {
				t.Skip("Windows file names cannot contain wildcard characters")
			}

			// Given
			parent := t.TempDir()
			dir := filepath.Join(parent, test.directory)
			sibling := filepath.Join(parent, test.sibling)
			expectNoErr(t, os.MkdirAll(dir, 0o700))
			expectNoErr(t, os.MkdirAll(sibling, 0o700))
			ownStale := filepath.Join(dir, "."+secretsFileName+".tmp-1")
			siblingTemporary := filepath.Join(sibling, "."+secretsFileName+".tmp-2")
			expectNoErr(t, os.WriteFile(ownStale, []byte("stale"), secretsFileMode))
			expectNoErr(t, os.WriteFile(siblingTemporary, []byte("sibling"), secretsFileMode))

			// When
			err := WriteSecrets(dir, &Secrets{DbPassword: "current"})

			// Then
			expectNoErr(t, err)
			assertOnlySecretsFile(t, dir)
			if _, statErr := os.Stat(siblingTemporary); statErr != nil {
				t.Fatalf("expected the other directory's file to be untouched, got %v", statErr)
			}
		})
	}
}
