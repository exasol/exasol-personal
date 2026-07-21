// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
)

var (
	terminalNotices []string
	terminalOutputs []string
)

func resetTerminalMessages() {
	terminalNotices = nil
	terminalOutputs = nil
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

func printTerminalMessages() {
	writeTerminalMessages(os.Stdout, os.Stderr)
}

func writeTerminalMessages(stdout, stderr io.Writer) {
	for _, message := range terminalNotices {
		_, _ = fmt.Fprintln(stderr, message)
	}
	terminalNotices = nil
	for _, message := range terminalOutputs {
		_, _ = fmt.Fprintln(stdout, message)
	}
	terminalOutputs = nil
}
