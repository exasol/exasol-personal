// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/exasol/exasol-personal/internal/connect/readline"
	"github.com/exasol/exasol-personal/internal/connect/types"
)

const exitCommand = "exit"

// ProcessInputFunc defines a way to process a shell input.
type ProcessInputFunc func(input string) error

type shell struct {
	lineReader   types.LineReader
	processInput ProcessInputFunc
}

func newShell(lineReader types.LineReader, processInput ProcessInputFunc) *shell {
	return &shell{lineReader, processInput}
}

func (sh *shell) close() error {
	return sh.lineReader.Close()
}

func (sh *shell) run() error {
	for {
		slog.Debug("reading next line")

		line, err := sh.lineReader.Readline()
		if err != nil {
			if errors.Is(err, types.ErrInterrupt) {
				continue
			} else if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}

		line = strings.TrimSpace(line)

		if line == exitCommand {
			slog.Debug("got the exit command, exitting")
			return nil
		}

		if err := sh.processInput(line); err != nil {
			slog.Error(err.Error())
		}
	}
}

func getHistoryFilePath() (string, error) {
	const historyFileName = "exasol_history"

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	historyFilePath := filepath.Join(cacheDir, historyFileName)

	slog.Debug("obtained history file path", "path", historyFilePath)

	return historyFilePath, nil
}

func runShellImpl(lineReader types.LineReader, processInput ProcessInputFunc) error {
	shell := newShell(lineReader, processInput)

	defer shell.close()

	return shell.run()
}

// RunShell runs the shell, processing incoming input
// with the passed callback. Blocks until the shell exits.
func RunShell(processInput ProcessInputFunc) error {
	historyFilePath, err := getHistoryFilePath()
	if err != nil {
		return fmt.Errorf("couldn't get the history file path: %w", err)
	}

	lineReader, err := readline.New(historyFilePath)
	if err != nil {
		return err
	}

	return runShellImpl(lineReader, processInput)
}
