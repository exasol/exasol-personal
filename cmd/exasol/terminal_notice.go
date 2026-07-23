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

// addTerminalCallToAction queues next-step guidance. Calls to action are
// suppressed under --json (see callsToActionVisible).
func addTerminalCallToAction(message string) {
	if message == "" {
		return
	}

	terminalCallsToAction = append(terminalCallsToAction, message)
}

func printTerminalMessages() {
	writeTerminalMessages(terminalConfig{
		stdout:            os.Stdout,
		stderr:            os.Stderr,
		showCallsToAction: callsToActionVisible(),
	})
}

// callsToActionVisible reports whether call-to-action guidance should be shown.
// Calls to action help any reader, including a non-interactive agent driving the
// CLI, so they are not TTY-gated; they are suppressed only under --json, where
// consumers branch on structured fields instead of prose.
func callsToActionVisible() bool {
	return !commonFlags.OutputJson
}

type terminalConfig struct {
	stdout            io.Writer
	stderr            io.Writer
	showCallsToAction bool
}

func writeTerminalMessages(cfg terminalConfig) {
	for _, message := range terminalNotices {
		_, _ = fmt.Fprintln(cfg.stderr, message)
	}
	terminalNotices = nil
	if cfg.showCallsToAction {
		for _, message := range terminalCallsToAction {
			_, _ = fmt.Fprintln(cfg.stderr, message)
		}
	}
	terminalCallsToAction = nil
	for _, message := range terminalOutputs {
		_, _ = fmt.Fprintln(cfg.stdout, message)
	}
	terminalOutputs = nil
}
