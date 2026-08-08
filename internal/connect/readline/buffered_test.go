// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package readline

import (
	"errors"
	"io"
	"strings"
	"testing"
)

const firstLine = "select 1;"

func TestBufferedReadlineStripsTrailingNewlineAndCarriageReturn(t *testing.T) {
	t.Parallel()

	reader := NewBuffered(strings.NewReader("select 1;\r\nselect 2;\n"))

	first, err := reader.Readline()
	if err != nil {
		t.Fatalf("expected first line to read, got %v", err)
	}
	if first != firstLine {
		t.Fatalf("expected CRLF trimmed, got %q", first)
	}

	second, err := reader.Readline()
	if err != nil {
		t.Fatalf("expected second line to read, got %v", err)
	}
	if second != "select 2;" {
		t.Fatalf("expected LF trimmed, got %q", second)
	}

	if _, err := reader.Readline(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after the last line, got %v", err)
	}
}

func TestBufferedReadlineReturnsFinalLineWithoutTrailingNewline(t *testing.T) {
	t.Parallel()

	// A script's last line commonly has no trailing newline; the reader must
	// still return it once (with EOF only on the following call) rather than
	// silently dropping it.
	reader := NewBuffered(strings.NewReader("select 1;\nselect 2"))

	first, err := reader.Readline()
	if err != nil {
		t.Fatalf("expected first line to read, got %v", err)
	}
	if first != firstLine {
		t.Fatalf("unexpected first line: %q", first)
	}

	second, err := reader.Readline()
	if err != nil {
		t.Fatalf("expected the unterminated final line to be returned, got error %v", err)
	}
	if second != "select 2" {
		t.Fatalf("expected final line without trailing newline, got %q", second)
	}

	if _, err := reader.Readline(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF once the final line is consumed, got %v", err)
	}
}

func TestBufferedReadlineOnEmptyInputReturnsEOFImmediately(t *testing.T) {
	t.Parallel()

	reader := NewBuffered(strings.NewReader(""))

	if _, err := reader.Readline(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF on empty input, got %v", err)
	}
}

func TestBufferedCloseIsANoop(t *testing.T) {
	t.Parallel()

	reader := NewBuffered(strings.NewReader("select 1;\n"))

	if err := reader.Close(); err != nil {
		t.Fatalf("expected Close to succeed, got %v", err)
	}

	// Close must not affect subsequent reads: the underlying bufio.Reader
	// owns no closable resource of its own.
	line, err := reader.Readline()
	if err != nil || line != firstLine {
		t.Fatalf("expected reads to still work after Close, got %q, %v", line, err)
	}
}
