// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package task_runner_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func DrainChannelNoBlock[T any](channel <-chan T, handler func(item T)) {
	for {
		select {
		case item := <-channel:
			handler(item)
		default:
			return
		}
	}
}

// BufferHandler writes raw log messages into a buffer.
type BufferHandler struct {
	buf     chan<- slog.Record
	testing *testing.T
}

func (*BufferHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true // accept all levels
}

func (h *BufferHandler) Handle(_ context.Context, record slog.Record) error {
	h.testing.Helper()

	if record.Level == slog.LevelDebug {
		_, err := os.Stderr.WriteString(record.Message)

		return err
	}

	select {
	case h.buf <- record:
	case <-time.After(2 * time.Second):
		h.testing.Fatal("failed to send log record")
	}

	return nil
}

func (h *BufferHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *BufferHandler) WithGroup(_ string) slog.Handler      { return h }

// Returns a retore function.
func ReplaceLogger(t *testing.T, buffer chan<- slog.Record) func() {
	t.Helper()

	originalLogger := slog.Default()

	testLogHandler := &BufferHandler{
		buf:     buffer,
		testing: t,
	}

	slog.SetDefault(slog.New(testLogHandler))

	return func() {
		slog.SetDefault(originalLogger) // restore
	}
}
