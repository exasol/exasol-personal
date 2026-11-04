// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import "strings"

type StartupError struct {
	Sidecar string
	// The command prints the cause; status reports the full diagnostic.
	Cause      string
	Diagnostic string
}

func NewStartupError(name, diagnostic, failure string) *StartupError {
	cause := failure
	full := failure
	if reason := ReportedCause(diagnostic); reason != "" {
		cause = failure + ": " + reason
		full = failure + ": " + diagnostic
	}

	return &StartupError{Sidecar: name, Cause: cause, Diagnostic: full}
}

func (failure *StartupError) Error() string {
	return "sidecar " + failure.Sidecar + ": " + failure.Cause
}

// ReportedCause prefers the runtime's own error line: a container runtime
// writes progress before it fails, so the first line is usually not the cause.
func ReportedCause(diagnostic string) string {
	var last string
	var reported []string
	for line := range strings.SplitSeq(diagnostic, "\n") {
		trimmed := strings.TrimSpace(line)
		// Docker follows its error with a pointer to the command's help.
		if trimmed == "" || strings.HasPrefix(trimmed, "Run '") {
			continue
		}
		last = trimmed
		if isErrorLine(trimmed) {
			reported = []string{trimmed}
		} else if len(reported) > 0 {
			reported = append(reported, trimmed)
		}
	}
	if len(reported) > 0 {
		return strings.Join(reported, "\n")
	}

	return last
}

// Docker's CLI may prefix the daemon's error with its own name.
func isErrorLine(line string) bool {
	lower := strings.TrimPrefix(strings.ToLower(line), "docker: ")

	return strings.HasPrefix(lower, "error:") ||
		strings.HasPrefix(lower, "error response from daemon:")
}
