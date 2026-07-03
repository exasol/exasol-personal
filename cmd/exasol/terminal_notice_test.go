// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"testing"
)

// TestTerminalMessagesPrintNoticesInQueueOrderAndOutputToStdout must not run in
// parallel: it mutates the package-level terminal message queues
// (resetTerminalMessages/addTerminal*/writeTerminalMessages), which are shared
// global state. Running alongside other tests that touch them (e.g.
// TestVersionCmdQueuesPrimaryOutputForTerminalFlush) trips the race detector.
//
//nolint:paralleltest // mutates shared package globals; must run serially
func TestTerminalMessagesPrintNoticesInQueueOrderAndOutputToStdout(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	addTerminalNotice("version notice")
	addTerminalNotice("command notice")
	addTerminalOutput("connection instructions")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(&stdout, &stderr)

	stdoutContent := stdout.String()
	if stdoutContent != "connection instructions\n" {
		t.Fatalf("unexpected stdout: %q", stdoutContent)
	}
	stderrContent := stderr.String()
	if stderrContent != "version notice\ncommand notice\n" {
		t.Fatalf("unexpected stderr: %q", stderrContent)
	}
}
