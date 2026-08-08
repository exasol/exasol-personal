// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/deploy"
)

func TestConfirmYes(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"y":       true,
		"Y":       true,
		"yes":     true,
		"YES":     true,
		"  yes  ": true,
		"n":       false,
		"no":      false,
		"":        false,
		"maybe":   false,
	}
	for input, want := range cases {
		if got := confirmYes(input); got != want {
			t.Errorf("confirmYes(%q) = %v, want %v", input, got, want)
		}
	}
}

// nolint: paralleltest // temporarily replaces the package-level os.Stdin.
func TestAskForUserConfirmation_ReadsAnswerFromStdin(t *testing.T) {
	restore := replaceStdinWithPipeContent(t, "yes\n")
	defer restore()

	if !askForUserConfirmation("") {
		t.Fatal("expected a 'yes' answer to confirm")
	}
}

// nolint: paralleltest // temporarily replaces the package-level os.Stdin.
func TestAskForUserConfirmation_RejectsNegativeAnswer(t *testing.T) {
	restore := replaceStdinWithPipeContent(t, "no\n")
	defer restore()

	if askForUserConfirmation("") {
		t.Fatal("expected a 'no' answer to not confirm")
	}
}

// nolint: paralleltest // temporarily replaces the package-level os.Stdin.
func TestAskForUserConfirmation_UsesCustomValidator(t *testing.T) {
	restore := replaceStdinWithPipeContent(t, "confirmed\n")
	defer restore()

	always := func(string) bool { return true }
	never := func(string) bool { return false }

	if !askForUserConfirmation("", always) {
		t.Fatal("expected the custom validator to be used")
	}

	restore2 := replaceStdinWithPipeContent(t, "confirmed\n")
	defer restore2()

	if askForUserConfirmation("", never) {
		t.Fatal("expected the custom validator to reject the answer")
	}
}

// nolint: paralleltest // temporarily replaces the package-level os.Stdin.
func TestAskForUserConfirmation_ReturnsFalseOnReadError(t *testing.T) {
	restore := replaceStdinWithPipeContent(t, "")
	defer restore()

	if askForUserConfirmation("") {
		t.Fatal("expected EOF on stdin to be treated as no confirmation")
	}
}

// replaceStdinWithPipeContent temporarily replaces os.Stdin with a pipe
// preloaded with content, so askForUserConfirmation's real bufio.Reader over
// os.Stdin can be exercised deterministically without a real terminal.
func replaceStdinWithPipeContent(t *testing.T, content string) func() {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	if _, err := writer.WriteString(content); err != nil {
		t.Fatalf("failed to write pipe content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}

	original := os.Stdin
	os.Stdin = reader

	return func() {
		os.Stdin = original
		_ = reader.Close()
	}
}

func TestLooksLikePathPresetArg(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg      string
		wantPath bool
	}{
		{"aws", false},
		{"ubuntu", false},
		{"my-preset", false},
		{"./local", true},
		{"/abs/path", true},
		{"~/home", true},
		{`C:\Windows\path`, true},
	}
	for _, tc := range cases {
		got := looksLikePathPresetArg(tc.arg)
		if got != tc.wantPath {
			t.Errorf("looksLikePathPresetArg(%q) = %v, want %v", tc.arg, got, tc.wantPath)
		}
	}
}

func TestLooksLikeExternalPresetURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		arg  string
		want bool
	}{
		{"file:///path", true},
		{"https://example.com/repo.git", true},
		{"http://example.com/repo.git", true},
		{"git://example.com/repo.git", true},
		{"git@github.com:org/repo.git", true},
		{"aws", false},
		{"./local", false},
		{"/abs/path", false},
		{"ubuntu", false},
	}
	for _, tc := range cases {
		got := deploy.IsExternalPresetURI(tc.arg)
		if got != tc.want {
			t.Errorf("IsExternalPresetURI(%q) = %v, want %v", tc.arg, got, tc.want)
		}
	}
}
