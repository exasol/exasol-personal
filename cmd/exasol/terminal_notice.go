// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
)

var (
	terminalNotices       []string
	terminalOutputs       []string
	terminalCallsToAction []string
)

func resetTerminalMessages() {
	terminalNotices = nil
	terminalOutputs = nil
	terminalCallsToAction = nil
}

func addTerminalNotice(message string) {
	if message == "" {
		return
	}

	terminalNotices = append(terminalNotices, message)
}

func addTerminalOutput(message string) {
	if message == "" {
		return
	}

	terminalOutputs = append(terminalOutputs, message)
}

// addTerminalCallToAction queues next-step guidance (a call to action). Such
// guidance is emitted on stderr, and is shown whenever call-to-action guidance
// is visible (see callToActionsVisible) so that it never lands in JSON output.
func addTerminalCallToAction(message string) {
	if message == "" {
		return
	}

	terminalCallsToAction = append(terminalCallsToAction, message)
}

func printTerminalMessages() {
	writeTerminalMessages(os.Stdout, os.Stderr, callToActionsVisible())
}

// callToActionsVisible reports whether call-to-action guidance should be shown.
// The guidance is textual next-step help that any reader benefits from,
// including a non-interactive agent driving the CLI in a workflow, so it is NOT
// gated on an interactive terminal. It is suppressed only under --json, where
// consumers want structured output and branch on structured state fields
// instead of prose.
func callToActionsVisible() bool {
	return !commonFlags.OutputJson
}

// showCallsToAction is a rendering mode (interactive vs. not), not internal
// control coupling; it is the seam that lets tests exercise both visibilities.
//
//nolint:revive // showCallsToAction selects the rendering mode for testability.
func writeTerminalMessages(stdout, stderr io.Writer, showCallsToAction bool) {
	for _, message := range terminalNotices {
		_, _ = fmt.Fprintln(stderr, message)
	}
	terminalNotices = nil
	if showCallsToAction {
		for _, message := range terminalCallsToAction {
			_, _ = fmt.Fprintln(stderr, message)
		}
	}
	terminalCallsToAction = nil
	for _, message := range terminalOutputs {
		_, _ = fmt.Fprintln(stdout, message)
	}
	terminalOutputs = nil
}
