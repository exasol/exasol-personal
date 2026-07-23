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
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	stdoutContent := stdout.String()
	if stdoutContent != "connection instructions\n" {
		t.Fatalf("unexpected stdout: %q", stdoutContent)
	}
	stderrContent := stderr.String()
	if stderrContent != "version notice\ncommand notice\n" {
		t.Fatalf("unexpected stderr: %q", stderrContent)
	}
}

// TestTerminalMessagesShowCallsToActionOnlyWhenVisible must not run in parallel:
// it mutates the package-level terminal message queues.
//
//nolint:paralleltest // mutates shared package globals; must run serially
func TestTerminalMessagesShowCallsToActionOnlyWhenVisible(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given: an operational notice, a call to action, and primary output.
	addTerminalNotice("directory notice")
	addTerminalCallToAction("run `exasol deploy`")
	addTerminalOutput("result")

	// When: calls to action are not visible (--json output).
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: false,
	})

	// Then: the notice and result remain, but the call to action is suppressed.
	if stdout.String() != "result\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "directory notice\n" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

// TestTerminalMessagesShowCallsToActionWhenVisible must not run in parallel: it
// mutates the package-level terminal message queues.
//
//nolint:paralleltest // mutates shared package globals; must run serially
func TestTerminalMessagesShowCallsToActionWhenVisible(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given: an operational notice and a call to action.
	addTerminalNotice("directory notice")
	addTerminalCallToAction("run `exasol deploy`")

	// When: calls to action are visible (interactive terminal).
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{stdout: &stdout, stderr: &stderr, showCallsToAction: true})

	// Then: the call to action follows the notice on stderr.
	if stdout.String() != "" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "directory notice\nrun `exasol deploy`\n" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

// TestCallToActionsVisibleFollowsJSONFlag must not run in parallel: it mutates
// the shared commonFlags global.
//
//nolint:paralleltest // mutates shared package globals; must run serially
func TestCallToActionsVisibleFollowsJSONFlag(t *testing.T) {
	old := commonFlags.OutputJson
	defer func() { commonFlags.OutputJson = old }()

	// Text output (interactive or not) shows guidance; only --json suppresses it.
	commonFlags.OutputJson = false
	if !callsToActionVisible() {
		t.Fatal("expected calls to action to be visible for text output")
	}
	commonFlags.OutputJson = true
	if callsToActionVisible() {
		t.Fatal("expected calls to action to be suppressed under --json")
	}
}
