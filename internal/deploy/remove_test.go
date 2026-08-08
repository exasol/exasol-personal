// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestRemoveDeploymentDirectoryRoot_RemovesEmptyDirectory(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	if err := removeDeploymentDirectoryRoot(deployment); err != nil {
		t.Fatalf("expected an empty directory to be removed, got %v", err)
	}
	if _, err := os.Stat(deployment.Root()); !os.IsNotExist(err) {
		t.Fatalf("expected the directory to no longer exist, got %v", err)
	}
}

func TestRemoveDeploymentDirectoryRoot_AlreadyRemovedIsNoop(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir() + "/already-gone")

	if err := removeDeploymentDirectoryRoot(deployment); err != nil {
		t.Fatalf("expected a missing directory to be a no-op, got %v", err)
	}
}

func TestRemoveDeploymentDirectoryRoot_NonEmptyDirectoryReturnsWrappedError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.WriteFile(deployment.Resolve("stray.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write stray file: %v", err)
	}

	err := removeDeploymentDirectoryRoot(deployment)
	if err == nil {
		t.Fatal("expected a non-empty directory to fail removal")
	}
}

func TestEnsureRemovableDeploymentDirectory_RejectsPathThatIsNotADirectory(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write file fixture: %v", err)
	}
	deployment := config.NewDeploymentDir(filePath)

	err := ensureRemovableDeploymentDirectory(deployment)
	if err == nil {
		t.Fatal("expected an error when the deployment root is a regular file")
	}
}

func TestPathIsInside(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()

	cases := map[string]struct {
		child string
		want  bool
	}{
		"nested path":    {child: filepath.Join(parent, "sub", "leaf"), want: true},
		"same path":      {child: parent, want: true},
		"sibling path":   {child: parent + "-sibling", want: false},
		"unrelated path": {child: filepath.Join(t.TempDir(), "other"), want: false},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := pathIsInside(parent, testCase.child); got != testCase.want {
				t.Errorf("pathIsInside(%q, %q) = %v, want %v",
					parent, testCase.child, got, testCase.want)
			}
		})
	}
}

func TestRegularFileExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write file fixture: %v", err)
	}

	exists, err := regularFileExists(filePath)
	if err != nil || !exists {
		t.Fatalf("expected an existing regular file to be reported, got exists=%v err=%v",
			exists, err)
	}

	exists, err = regularFileExists(filepath.Join(dir, "missing.txt"))
	if err != nil || exists {
		t.Fatalf("expected a missing file to be reported absent, got exists=%v err=%v",
			exists, err)
	}

	exists, err = regularFileExists(dir)
	if err != nil || exists {
		t.Fatalf("expected a directory to not count as a regular file, got exists=%v err=%v",
			exists, err)
	}
}
