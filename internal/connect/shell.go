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

type ShellOpts struct {
	ExecuteOnSemicolon bool
}

type shell struct {
	lineReader          types.LineReader
	processInput        ProcessInputFunc
	executeOnSemicolon  bool
	pendingStatementBuf string
}

func newShell(lineReader types.LineReader, processInput ProcessInputFunc, opts ShellOpts) *shell {
	return &shell{
		lineReader:         lineReader,
		processInput:       processInput,
		executeOnSemicolon: opts.ExecuteOnSemicolon,
	}
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

		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == exitCommand && strings.TrimSpace(sh.pendingStatementBuf) == "" {
			slog.Debug("got the exit command, exitting")
			return nil
		}

		if !sh.executeOnSemicolon {
			if err := sh.processInput(trimmedLine); err != nil {
				slog.Error(err.Error())
			}

			continue
		}

		if err := sh.processInputSemicolonMode(line); err != nil {
			slog.Error(err.Error())
		}
	}
}

func (sh *shell) processInputSemicolonMode(line string) error {
	if sh.pendingStatementBuf != "" {
		sh.pendingStatementBuf += "\n"
	}
	sh.pendingStatementBuf += line

	statements, remainder := splitSemicolonTerminatedStatements(sh.pendingStatementBuf)
	sh.pendingStatementBuf = remainder

	for _, statement := range statements {
		if err := sh.processInput(strings.TrimSpace(statement)); err != nil {
			return err
		}
	}

	return nil
}

func splitSemicolonTerminatedStatements(sql string) ([]string, string) {
	var (
		statements     []string
		start          int
		inSingleQuotes bool
	)

	for charIndex := 0; charIndex < len(sql); charIndex++ {
		switch sql[charIndex] {
		case '\'':
			if inSingleQuotes && charIndex+1 < len(sql) && sql[charIndex+1] == '\'' {
				charIndex++
				continue
			}
			inSingleQuotes = !inSingleQuotes
		case ';':
			if inSingleQuotes {
				continue
			}
			statement := strings.TrimSpace(sql[start:charIndex])
			if statement != "" {
				statements = append(statements, statement)
			}
			start = charIndex + 1
		default:
			continue
		}
	}

	return statements, sql[start:]
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

func runShellImpl(
	lineReader types.LineReader,
	processInput ProcessInputFunc,
	opts ShellOpts,
) error {
	shell := newShell(lineReader, processInput, opts)

	defer shell.close()

	return shell.run()
}

// RunShell runs the shell, processing incoming input
// with the passed callback. Blocks until the shell exits.
func RunShell(processInput ProcessInputFunc) error {
	return RunShellWithOpts(processInput, ShellOpts{ExecuteOnSemicolon: true})
}

func RunShellWithOpts(processInput ProcessInputFunc, opts ShellOpts) error {
	historyFilePath, err := getHistoryFilePath()
	if err != nil {
		return fmt.Errorf("couldn't get the history file path: %w", err)
	}

	lineReader, err := readline.New(historyFilePath)
	if err != nil {
		return err
	}

	return runShellImpl(lineReader, processInput, opts)
}
