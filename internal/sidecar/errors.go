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
	if reason := reportedCause(diagnostic); reason != "" {
		cause = failure + ": " + reason
		full = failure + ": " + diagnostic
	}

	return &StartupError{Sidecar: name, Cause: cause, Diagnostic: full}
}

func (failure *StartupError) Error() string {
	return "sidecar " + failure.Sidecar + ": " + failure.Cause
}

// reportedCause prefers the runtime's own error line: a container runtime
// writes progress before it fails, so the first line is usually not the cause.
func reportedCause(diagnostic string) string {
	var last, reported string
	for line := range strings.SplitSeq(diagnostic, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		last = trimmed
		if strings.HasPrefix(strings.ToLower(trimmed), "error:") {
			reported = trimmed
		}
	}
	if reported != "" {
		return reported
	}

	return last
}
