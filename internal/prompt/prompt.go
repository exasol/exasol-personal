// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package prompt provides minimal terminal-detection and yes/no prompt
// helpers for launcher lifecycle steps that need optional user consent.
//
// Callers pass an io.Reader for stdin. If the reader is not a TTY (nil,
// os.DevNull, a piped stdin, etc.) YesNo returns the supplied default
// without printing anything, so the same code path serves both
// interactive and scripted invocations.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// IsTerminal reports whether in is a real terminal file descriptor.
// Non-*os.File readers (bytes.Buffer, strings.Reader, nil) return false.
func IsTerminal(in io.Reader) bool {
	if in == nil {
		return false
	}
	file, ok := in.(*os.File)
	if !ok {
		return false
	}

	return term.IsTerminal(int(file.Fd()))
}

// YesNo prints question with a "[Y/n]" or "[y/N]" hint (per defaultYes)
// and reads one line from in. Empty input yields the default. The first
// character is matched case-insensitively; anything other than y/n also
// yields the default. Returns the default silently and without prompting
// when in is not a terminal.
func YesNo(in io.Reader, out io.Writer, question string, defaultYes bool) (bool, error) {
	if !IsTerminal(in) {
		return defaultYes, nil
	}
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	if _, err := fmt.Fprintf(out, "%s %s ", question, hint); err != nil {
		return defaultYes, fmt.Errorf("could not write prompt: %w", err)
	}
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return defaultYes, fmt.Errorf("could not read prompt response: %w", err)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return defaultYes, nil
	}
	switch strings.ToLower(trimmed[:1]) {
	case "y":
		return true, nil
	case "n":
		return false, nil
	}

	return defaultYes, nil
}
