// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // Not parallel due to overriding slog
func TestLogBuffer(t *testing.T) {
	logBuffer := &task_runner.LogBuffer{}

	before := time.Now()
	var err error

	_, err = logBuffer.Write([]byte("0\n1"))
	require.NoError(t, err)

	_, err = logBuffer.Write([]byte("\n2\n"))
	require.NoError(t, err)

	_, err = logBuffer.Write([]byte("3"))
	require.NoError(t, err)

	after := time.Now()

	logsChan := make(chan slog.Record, 100)
	restore := ReplaceLogger(t, logsChan)
	defer restore()

	logBuffer.ReplayLogMessages(t.Context())

	logMessages := make([]string, 4)
	idx := 0

	DrainChannelNoBlock(logsChan, func(record slog.Record) {
		if idx >= len(logMessages) {
			require.FailNow(t, "received too many slog messages")
		}

		require.True(t, record.Time.After(before))
		require.True(t, record.Time.Before(after))

		logMessages[idx] = record.Message

		idx++
	})

	require.Equal(t, "0", logMessages[0])
	require.Equal(t, "1", logMessages[1])
	require.Equal(t, "2", logMessages[2])
	require.Equal(t, "3", logMessages[3])
}
