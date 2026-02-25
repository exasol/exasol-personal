// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build !windows

package remote

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func handleTerminalResize(session *ssh.Session) func() {
	// Handle terminal resize
	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGWINCH)

	stdInFd := int(os.Stdin.Fd())

	go func() {
		for range sigs {
			width, height, _ := term.GetSize(stdInFd)

			err := session.WindowChange(height, width)
			if err != nil {
				slog.Error("failed to handle terminal window resize", "error", err.Error())
			}
		}
	}()

	return func() {
		close(sigs)
	}
}
