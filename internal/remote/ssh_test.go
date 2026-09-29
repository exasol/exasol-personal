// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package remote

import "testing"

func TestShellCommandPreservesArguments(t *testing.T) {
	t.Parallel()

	// Given
	args := []string{"printf", "%s\\n", "two words", "it's", "", "$HOME"}

	// When
	actual := shellCommand(args)

	// Then
	want := `'printf' '%s\n' 'two words' 'it'"'"'s' '' '$HOME'`
	if actual != want {
		t.Fatalf("shell command = %q, want %q", actual, want)
	}
}
