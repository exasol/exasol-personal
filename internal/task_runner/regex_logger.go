// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner

import (
	"bytes"
	"io"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/presets"
)

type RegexLogger interface {
	Write(data []byte) (int, error)
}

var _ io.Writer = (RegexLogger)(nil)

type regexLogger struct {
	patterns   []*presets.RegexLog
	lineBuffer bytes.Buffer
	nodePrefix string
}

func NewRegexLogger(patterns []*presets.RegexLog) RegexLogger {
	return &regexLogger{
		patterns:   patterns,
		nodePrefix: "",
	}
}

func NewRegexLoggerWithNode(patterns []*presets.RegexLog, nodeName string) RegexLogger {
	return &regexLogger{
		patterns:   patterns,
		nodePrefix: "host(" + nodeName + "): ",
	}
}

// Write adds the data to a buffer. When the buffer contains a full line, it runs
// regexes against it and sends log messages if they match. The buffer is then cleared
// for the next line.
func (w *regexLogger) Write(data []byte) (int, error) {
	count := 0 // count tracks how many bytes we have processed

	for {
		startLen := len(data)

		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			tmpCount, err := w.lineBuffer.Write(data)
			count += tmpCount

			return count, err
		}

		tmpCount, err := w.lineBuffer.Write(data[:idx+1])
		count += tmpCount

		if err != nil {
			return count, err
		}

		w.logMatchingPatterns()

		w.lineBuffer.Reset()
		data = data[idx+1:]

		if len(data) >= startLen {
			panic("expected remaining bytes to decrease")
		}
	}
}

// logMatchingPatterns runs every pattern against the current line buffer and
// logs a message for each one that matches.
func (w *regexLogger) logMatchingPatterns() {
	for _, pattern := range w.patterns {
		matches := pattern.CompiledRegex.FindSubmatch(w.lineBuffer.Bytes())
		if matches == nil {
			continue
		}

		w.logMatch(pattern, matches)
	}
}

// logMatch renders a pattern's message template against the captured groups
// and emits it at the configured log level.
func (w *regexLogger) logMatch(pattern *presets.RegexLog, matches [][]byte) {
	// Replace placeholders in message with captured groups
	// $0 = full match, $1 = first group, $2 = second group, etc.
	logMsg := []byte(pattern.Message)
	for i, match := range matches {
		placeholder := "$" + string(rune('0'+i))
		matchStr := string(bytes.TrimRight(match, "\n\r"))
		logMsg = bytes.ReplaceAll(logMsg, []byte(placeholder), []byte(matchStr))
	}

	if w.nodePrefix != "" {
		logMsg = append([]byte(w.nodePrefix), logMsg...)
	}

	if pattern.LogAsError {
		slog.Error(string(logMsg))
	} else {
		slog.Info(string(logMsg))
	}
}
