// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import "strings"

// StartupError separates the short cause a command prints from the full
// diagnostic status reports, so every startup failure reads the same way
// whether the port was taken, the image was missing, or the entrypoint failed.
type StartupError struct {
	Sidecar    string
	Cause      string
	Diagnostic string
}

// NewStartupError pairs the runtime's own failure with the first meaningful
// line of its diagnostic, which is where container runtimes report the reason.
func NewStartupError(name, diagnostic, failure string) *StartupError {
	cause := failure
	full := failure
	if reason := firstMeaningfulLine(diagnostic); reason != "" {
		cause = failure + ": " + reason
		full = failure + ": " + diagnostic
	}

	return &StartupError{Sidecar: name, Cause: cause, Diagnostic: full}
}

func (failure *StartupError) Error() string {
	return "sidecar " + failure.Sidecar + ": " + failure.Cause
}

func firstMeaningfulLine(diagnostic string) string {
	for line := range strings.SplitSeq(diagnostic, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}

	return ""
}
