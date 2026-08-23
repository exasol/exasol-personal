// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsTerminal_NilAndBufferReturnFalse(t *testing.T) {
	if IsTerminal(nil) {
		t.Errorf("nil reader must not be a terminal")
	}
	if IsTerminal(&bytes.Buffer{}) {
		t.Errorf("bytes.Buffer must not be a terminal")
	}
	if IsTerminal(strings.NewReader("")) {
		t.Errorf("strings.Reader must not be a terminal")
	}
}

func TestYesNo_NonTerminalReturnsDefaultSilently(t *testing.T) {
	in := strings.NewReader("y\n")
	var out bytes.Buffer

	got, err := YesNo(in, &out, "Proceed?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Errorf("non-terminal path must return the default (false)")
	}
	if out.Len() != 0 {
		t.Errorf("non-terminal path must not print anything, got %q", out.String())
	}
}

func TestYesNo_NonTerminalDefaultYesReturnsYes(t *testing.T) {
	got, err := YesNo(strings.NewReader(""), &bytes.Buffer{}, "Proceed?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("non-terminal path must return the default (true)")
	}
}

func TestYesNo_NilReaderReturnsDefault(t *testing.T) {
	got, err := YesNo(nil, &bytes.Buffer{}, "Proceed?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("nil reader must return default")
	}
}
