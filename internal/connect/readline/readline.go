// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package readline

import (
	"errors"
	"fmt"

	"github.com/chzyer/readline"
	"github.com/exasol/exasol-personal/internal/connect/types"
)

type Readline struct {
	*readline.Instance
}

func New(historyFilePath string) (types.LineReader, error) {
	rline, err := readline.NewEx(&readline.Config{
		Prompt:            "\033[32m>\033[0m ", // Green ">" character
		HistoryFile:       historyFilePath,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		return nil, err
	}

	rline.CaptureExitSignal()

	return &Readline{rline}, nil
}

func (rl *Readline) Readline() (string, error) {
	line, err := rl.Instance.Readline()
	if err != nil {
		// Instead of returning readline-specific error
		// we return our own generic error type.
		if errors.Is(err, readline.ErrInterrupt) {
			return "", fmt.Errorf("%w: %s", types.ErrInterrupt, err) // nolint: errorlint
		}

		return "", err
	}

	return line, nil
}
