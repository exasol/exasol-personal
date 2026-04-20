// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/term"
)

var ErrNotImplemented = errors.New("not implemented")

var ErrPathIsNotDir = errors.New("path is not a directory")

func EnsureDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		const dirPerm = 0o750
		return os.MkdirAll(path, dirPerm)
	}

	if err != nil {
		return err
	}

	if !info.IsDir() {
		return LoggedError(ErrPathIsNotDir, "path", path)
	}

	return nil
}

var ErrIsNotDirectory = errors.New("is not a directory")

func ListDir(path string, maxEntries int) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, LoggedError(ErrPathIsNotDir, "", "path", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	entries, err := file.Readdirnames(maxEntries)
	if errors.Is(err, io.EOF) {
		return entries, nil
	}

	return entries, err
}

func AbsPathNoFail(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	return absPath
}

type Optional[T any] struct {
	value   T
	present bool
}

func New[T any](v T) Optional[T] {
	return Optional[T]{v, true}
}

func Nothing[T any]() Optional[T] {
	return Optional[T]{present: false}
}

func (o Optional[T]) Unwrap() (T, bool) {
	return o.value, o.present
}

// Helper function to log and wrap an error in one step.
func LoggedError(err error, context string, args ...any) error {
	if context == "" {
		slog.Error(err.Error(), args...)
	} else {
		slog.Error(fmt.Sprintf("%s: %s", err.Error(), context), args...)
	}

	msg := context
	if len(args) > 0 {
		msg = context + ": "
		for i, arg := range args {
			if i%2 == 0 { //nolint: revive
				msg += fmt.Sprintf("%v=", arg)
			} else {
				msg += fmt.Sprintf("\"%v\" ", arg)
			}
		}
	}

	return fmt.Errorf("%w: %s", err, msg)
}

func CombineWriters(first, second io.Writer) io.Writer {
	if first == nil {
		return second
	}

	if second == nil {
		return first
	}

	return io.MultiWriter(first, second)
}

// GetTerminalWidth returns the current terminal
// width with a boolean signifying if the width
// could be obtained.
func GetTerminalWidth() (int, bool) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		slog.Debug(err.Error())
		return 0, false
	}

	return width, true
}

// IsInteractiveStdin returns true when stdin is attached to a terminal.
func IsInteractiveStdin() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
}

var (
	ErrSourceNotDir = errors.New("source is not a directory")
	ErrDestNotDir   = errors.New("destination is not a directory")
	ErrCopyFailed   = errors.New("copy operation failed")
)

type CopyDirError struct {
	Op  string
	Src string
	Dst string
	Err error
}

func (e *CopyDirError) Error() string {
	switch {
	case e.Src != "" && e.Dst != "":
		return fmt.Sprintf("%s (%s -> %s): %v", e.Op, e.Src, e.Dst, e.Err)
	case e.Src != "":
		return fmt.Sprintf("%s (%s): %v", e.Op, e.Src, e.Err)
	default:
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}
}

func (e *CopyDirError) Unwrap() error {
	return e.Err
}

func CopyDir(src string, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return &CopyDirError{
			Op:  "stat source",
			Src: src,
			Err: err,
		}
	}
	if !srcInfo.IsDir() {
		return &CopyDirError{
			Op:  "validate source",
			Src: src,
			Err: ErrSourceNotDir,
		}
	}

	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return &CopyDirError{
			Op:  "create destination",
			Src: src,
			Dst: dst,
			Err: err,
		}
	}

	dstInfo, err := os.Stat(dst)
	if err != nil {
		return &CopyDirError{
			Op:  "stat destination",
			Dst: dst,
			Err: err,
		}
	}
	if !dstInfo.IsDir() {
		return &CopyDirError{
			Op:  "validate destination",
			Dst: dst,
			Err: ErrDestNotDir,
		}
	}

	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return &CopyDirError{
			Op:  "copy directory",
			Src: src,
			Dst: dst,
			Err: fmt.Errorf("%w: %w", ErrCopyFailed, err),
		}
	}

	return nil
}
