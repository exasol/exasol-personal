// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package readline

import (
	"bufio"
	"io"
	"strings"

	"github.com/exasol/exasol-personal/internal/connect/types"
)

type Buffered struct {
	reader *bufio.Reader
}

func NewBuffered(input io.Reader) types.LineReader {
	return &Buffered{
		reader: bufio.NewReader(input),
	}
}

func (reader *Buffered) Readline() (string, error) {
	line, err := reader.reader.ReadString('\n')
	if err != nil {
		if err == io.EOF && line != "" {
			return strings.TrimRight(line, "\r\n"), nil
		}

		return "", err
	}

	return strings.TrimRight(line, "\r\n"), nil
}

func (*Buffered) Close() error {
	return nil
}
