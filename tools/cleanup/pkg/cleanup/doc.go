// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package cleanup exposes the internal Exasol Personal cleanup domain model and
// orchestration helpers for Go automation.
//
// The package does not write to terminals, parse command-line flags, or format
// tables. Callers provide provider collectors, receive typed scope and cleanup
// results, and decide how to render or persist those results.
package cleanup
