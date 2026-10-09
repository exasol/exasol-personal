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

//nolint:paralleltest // mutates shared terminal message queues
func TestStartupFlushDrainsAllMessageKindsBeforeLaterMessages(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given messages queued during shared startup.
	addTerminalNotice("Using default deployment directory: /deployments/default")
	addTerminalOutput("startup result")
	addTerminalCallToAction("startup guidance")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// When shared startup flushes all queues and the command later queues messages.
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: true,
	})
	if stdout.String() != "startup result\n" || stderr.String() !=
		"Using default deployment directory: /deployments/default\n\nstartup guidance\n" {
		t.Fatalf("startup messages not flushed before command: stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
	addTerminalOutput("command result")
	addTerminalCallToAction("run `exasol connect`")
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: true,
	})

	// Then startup messages are not printed again and later guidance is last.
	if stdout.String() != "startup result\ncommand result\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	expected := "Using default deployment directory: /deployments/default\n\n" +
		"startup guidance\n\nrun `exasol connect`\n"
	if stderr.String() != expected {
		t.Fatalf("unexpected stderr: %q", stderr.String())
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
	if stderr.String() != "directory notice\n\nrun `exasol deploy`\n" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestCallToActionOnlyBatchStartsWithOneBlankLine(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	addTerminalCallToAction("run `exasol init`")
	addTerminalCallToAction("then run `exasol deploy`")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: true,
	})
	writeTerminalMessages(terminalConfig{
		stdout: &stdout, stderr: &stderr, showCallsToAction: true,
	})

	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.String() != "\nrun `exasol init`\nthen run `exasol deploy`\n" {
		t.Fatalf("unexpected CTA-only output: %q", stderr.String())
	}
}

// TestTerminalMessagesPrintCallsToActionLastInCombinedTerminalOutput must not
// run in parallel: it mutates the package-level terminal message queues.
//
//nolint:paralleltest // mutates shared package globals; must run serially
func TestTerminalMessagesPrintCallsToActionLastInCombinedTerminalOutput(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	// Given: all output kinds are queued.
	addTerminalNotice("directory notice")
	addTerminalCallToAction("run `exasol deploy`")
	addTerminalOutput("deployment overview")

	// When: stdout and stderr are rendered to the same terminal.
	terminal := bytes.Buffer{}
	writeTerminalMessages(terminalConfig{
		stdout: &terminal, stderr: &terminal, showCallsToAction: true,
	})

	// Then: the call to action is visually separated and printed last.
	expected := "directory notice\ndeployment overview\n\nrun `exasol deploy`\n"
	if terminal.String() != expected {
		t.Fatalf("unexpected terminal output: %q", terminal.String())
	}
}

func TestCallsToActionVisible(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		jsonOutput bool
		expected   bool
	}{
		{name: "text output", jsonOutput: false, expected: true},
		{name: "JSON output", jsonOutput: true, expected: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if callsToActionVisible(test.jsonOutput) != test.expected {
				t.Fatalf("unexpected visibility for JSON output %t", test.jsonOutput)
			}
		})
	}
}

//nolint:paralleltest // mutates shared terminal message queues
func TestWriteTerminalCallsToActionFollowsCommandError(t *testing.T) {
	resetTerminalMessages()
	defer resetTerminalMessages()

	addTerminalCallToAction("run `exasol config set`")
	stderr := bytes.NewBufferString("Error: port unavailable\n")
	writeTerminalCallsToAction(stderr, true)

	if stderr.String() != "Error: port unavailable\n\nrun `exasol config set`\n" {
		t.Fatalf("unexpected error and call-to-action output: %q", stderr.String())
	}
}
