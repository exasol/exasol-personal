// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

//go:build windows

package remote

import (
	"log/slog"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func handleTerminalResize(session *ssh.Session) func() {
	// Handle terminal resize by polling
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Unfortunately we can't use stdin to determine terminal size on
		// Windows, we must use stdout.
		stdOutFd := int(os.Stdout.Fd())

		prevWidth := 0
		prevHeight := 0

		for {
			select {
			case <-done:
				return
			default:
			}

			width, height, _ := term.GetSize(stdOutFd)
			if width != prevWidth || height != prevHeight {
				err := session.WindowChange(height, width)
				if err != nil {
					slog.Error("failed to handle terminal window resize", "error", err.Error())
				}
			}

			prevWidth = width
			prevHeight = height

			time.Sleep(500 * time.Millisecond)
		}
	}()

	return func() {
		done <- struct{}{}
	}
}
