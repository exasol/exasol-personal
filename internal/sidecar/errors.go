// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/localports"
)

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

type PortConflictError struct {
	Sidecar  string
	Port     string
	HostPort int
	Cause    error
}

func (failure *PortConflictError) Error() string {
	return fmt.Sprintf(
		"sidecar %s cannot bind host port %d of port %s: %v",
		failure.Sidecar, failure.HostPort, failure.Port, failure.Cause,
	)
}

func (failure *PortConflictError) Unwrap() error { return failure.Cause }

// ClassifyPortConflict identifies the conflicting port by the host port the
// diagnostic names, or by the only published one.
func ClassifyPortConflict(name string, ports []Port, cause error, diagnostic string) error {
	if !localports.BindConflict(diagnostic) {
		return cause
	}
	// Recovery selects a port by name, so an unnamed one stays a startup failure.
	published := slices.DeleteFunc(slices.Clone(ports), func(port Port) bool {
		return port.HostPort == 0 || port.Name == ""
	})
	for _, port := range published {
		if len(published) == 1 ||
			strings.Contains(diagnostic, ":"+strconv.Itoa(port.HostPort)) {
			return &PortConflictError{
				Sidecar: name, Port: port.Name, HostPort: port.HostPort, Cause: cause,
			}
		}
	}

	return cause
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
