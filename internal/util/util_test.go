// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDirCreatesMissingDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "dir")

	if err := EnsureDir(path); err != nil {
		t.Fatalf("expected directory creation to succeed, got %v", err)
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected %s to exist as a directory, got info=%v err=%v", path, info, err)
	}
}

func TestEnsureDirIsANoopWhenDirectoryAlreadyExists(t *testing.T) {
	t.Parallel()

	path := t.TempDir()

	if err := EnsureDir(path); err != nil {
		t.Fatalf("expected no-op success on an existing directory, got %v", err)
	}
}

func TestEnsureDirRejectsAnExistingNonDirectoryPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	err := EnsureDir(path)

	if !errors.Is(err, ErrPathIsNotDir) {
		t.Fatalf("expected ErrPathIsNotDir, got %v", err)
	}
}

func TestListDirReturnsEntryNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	entries, err := ListDir(dir, -1)
	if err != nil {
		t.Fatalf("expected listing to succeed, got %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", entries)
	}
}

func TestListDirRespectsMaxEntriesWithoutErroringAtTheLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	entries, err := ListDir(dir, 2)
	if err != nil {
		t.Fatalf("expected a bounded listing to succeed (io.EOF is swallowed), got %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected exactly 2 entries, got %v", entries)
	}
}

func TestListDirRejectsANonDirectoryPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := ListDir(path, -1)

	if !errors.Is(err, ErrPathIsNotDir) {
		t.Fatalf("expected ErrPathIsNotDir, got %v", err)
	}
}

func TestListDirPropagatesStatErrorForMissingPath(t *testing.T) {
	t.Parallel()

	_, err := ListDir(filepath.Join(t.TempDir(), "missing"), -1)

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestAbsPathNoFailReturnsAbsolutePath(t *testing.T) {
	t.Parallel()

	if !filepath.IsAbs(AbsPathNoFail("relative/path")) {
		t.Fatal("expected an absolute path")
	}

	absolute := filepath.Join(t.TempDir(), "already-absolute")
	if AbsPathNoFail(absolute) != absolute {
		t.Fatalf("expected an already-absolute path to be returned unchanged, got %q",
			AbsPathNoFail(absolute))
	}
}

func TestOptionalUnwrapsPresentAndAbsentValues(t *testing.T) {
	t.Parallel()

	value, isPresent := New(42).Unwrap()
	if !isPresent || value != 42 {
		t.Fatalf("expected (42, true), got (%v, %v)", value, isPresent)
	}

	value, isPresent = Nothing[int]().Unwrap()
	if isPresent || value != 0 {
		t.Fatalf("expected (0, false), got (%v, %v)", value, isPresent)
	}
}

func TestLoggedErrorWrapsWithoutContext(t *testing.T) {
	t.Parallel()

	base := errors.New("boom")

	err := LoggedError(base, "")

	if !errors.Is(err, base) {
		t.Fatalf("expected wrapped error to satisfy errors.Is(base), got %v", err)
	}
}

func TestLoggedErrorWrapsWithContextAndKeyValueArgs(t *testing.T) {
	t.Parallel()

	base := errors.New("boom")

	err := LoggedError(base, "operation failed", "path", "/tmp/x")

	if !errors.Is(err, base) {
		t.Fatalf("expected wrapped error to satisfy errors.Is(base), got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"boom", "operation failed", "path=", `"/tmp/x"`} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected message to contain %q, got %q", want, msg)
		}
	}
}

func TestCombineWritersHandlesNilAndBothSet(t *testing.T) {
	t.Parallel()

	var first, second bytes.Buffer

	if CombineWriters(nil, nil) != nil {
		t.Fatal("expected nil when both writers are nil")
	}
	if CombineWriters(&first, nil) != io.Writer(&first) {
		t.Fatal("expected the first writer to be returned when second is nil")
	}
	if CombineWriters(nil, &second) != io.Writer(&second) {
		t.Fatal("expected the second writer to be returned when first is nil")
	}

	combined := CombineWriters(&first, &second)
	if _, err := combined.Write([]byte("hello")); err != nil {
		t.Fatalf("expected combined write to succeed, got %v", err)
	}
	if first.String() != "hello" || second.String() != "hello" {
		t.Fatalf("expected both writers to receive the data, got %q and %q",
			first.String(), second.String())
	}
}

func TestGetTerminalWidthReturnsFalseWhenStdoutIsNotATerminal(t *testing.T) {
	t.Parallel()

	// go test redirects stdout away from a real terminal, so this exercises
	// the real, common non-interactive path deterministically.
	if _, ok := GetTerminalWidth(); ok {
		t.Skip("stdout is unexpectedly a terminal in this environment")
	}
}

func TestIsInteractiveStdinReturnsFalseWhenStdinIsNotATerminal(t *testing.T) {
	t.Parallel()

	// go test redirects stdin away from a real terminal, so this exercises
	// the real, common non-interactive path deterministically.
	if IsInteractiveStdin() {
		t.Skip("stdin is unexpectedly a terminal in this environment")
	}
}

func TestCopyDirErrorFormatsBySpecificity(t *testing.T) {
	t.Parallel()

	base := errors.New("underlying")

	cases := map[string]struct {
		err  *CopyDirError
		want string
	}{
		"src and dst": {
			err:  &CopyDirError{Op: "copy", Src: "/a", Dst: "/b", Err: base},
			want: "copy (/a -> /b): underlying",
		},
		"src only": {
			err:  &CopyDirError{Op: "stat source", Src: "/a", Err: base},
			want: "stat source (/a): underlying",
		},
		"neither": {
			err:  &CopyDirError{Op: "copy", Err: base},
			want: "copy: underlying",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.err.Error(); got != testCase.want {
				t.Fatalf("%s: got %q, want %q", name, got, testCase.want)
			}
			if !errors.Is(testCase.err, base) {
				t.Fatalf("%s: expected Unwrap to expose the underlying error", name)
			}
		})
	}
}

func TestCopyDirCopiesFilesAndSubdirectories(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o750); err != nil {
		t.Fatalf("failed to create source subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("top"), 0o600); err != nil {
		t.Fatalf("failed to write top-level file: %v", err)
	}
	nestedPath := filepath.Join(src, "sub", "nested.txt")
	if err := os.WriteFile(nestedPath, []byte("nested"), 0o600); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "copy-target")

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("expected copy to succeed, got %v", err)
	}

	top, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	if err != nil || string(top) != "top" {
		t.Fatalf("expected top-level file to be copied, got %q, err %v", top, err)
	}
	nested, err := os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
	if err != nil || string(nested) != "nested" {
		t.Fatalf("expected nested file to be copied, got %q, err %v", nested, err)
	}
}

func TestCopyDirRejectsNonexistentSource(t *testing.T) {
	t.Parallel()

	err := CopyDir(filepath.Join(t.TempDir(), "missing"), t.TempDir())

	var copyErr *CopyDirError
	if !errors.As(err, &copyErr) || copyErr.Op != "stat source" {
		t.Fatalf("expected a 'stat source' CopyDirError, got %v", err)
	}
}

func TestCopyDirRejectsNonDirectorySource(t *testing.T) {
	t.Parallel()

	src := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	err := CopyDir(src, t.TempDir())

	if !errors.Is(err, ErrSourceNotDir) {
		t.Fatalf("expected ErrSourceNotDir, got %v", err)
	}
}

// A destination that already exists as a plain file is rejected by the
// os.MkdirAll call itself (it cannot create a directory where a file already
// sits), surfacing as a "create destination" CopyDirError rather than ever
// reaching the later ErrDestNotDir check.
func TestCopyDirRejectsDestinationThatIsAFile(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "dst-is-a-file")
	if err := os.WriteFile(dst, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("failed to write destination file: %v", err)
	}

	err := CopyDir(src, dst)

	var copyErr *CopyDirError
	if !errors.As(err, &copyErr) || copyErr.Op != "create destination" {
		t.Fatalf("expected a 'create destination' CopyDirError, got %v", err)
	}
}
