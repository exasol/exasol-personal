// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

type testTerminalHandler struct {
	minLevel slog.Level
	messages []string
}

func (h *testTerminalHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *testTerminalHandler) Handle(_ context.Context, record slog.Record) error {
	h.messages = append(h.messages, record.Message)
	return nil
}

func (h *testTerminalHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *testTerminalHandler) WithGroup(_ string) slog.Handler {
	return h
}

func TestRoutingHandler_DoesNotLeakDebugToTerminal(t *testing.T) {
	t.Parallel()

	terminal := &testTerminalHandler{minLevel: slog.LevelInfo}
	fileBuffer := &bytes.Buffer{}
	sink := &deploymentFileSink{}
	sink.Set(fileBuffer, slog.LevelDebug)

	handler := newRoutingHandler(terminal, sink)

	debugRecord := slog.NewRecord(
		time.Now().UTC(),
		slog.LevelDebug,
		"debug message",
		0,
	)
	if err := handler.Handle(context.Background(), debugRecord); err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}

	if len(terminal.messages) != 0 {
		t.Fatalf("expected terminal to skip debug records, got: %#v", terminal.messages)
	}
	if !bytes.Contains(fileBuffer.Bytes(), []byte("[DEBUG] debug message")) {
		t.Fatalf("expected file sink to include debug message, got: %q", fileBuffer.String())
	}
}

func TestRoutingHandler_DebugRecordWritesToFileSink(t *testing.T) {
	t.Parallel()

	terminal := &testTerminalHandler{minLevel: slog.LevelDebug}
	fileBuffer := &bytes.Buffer{}
	sink := &deploymentFileSink{}
	sink.Set(fileBuffer, slog.LevelDebug)

	handler := newRoutingHandler(terminal, sink)

	record := slog.NewRecord(
		time.Now().UTC(),
		slog.LevelDebug,
		"normal debug message",
		0,
	)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}

	if !bytes.Contains(fileBuffer.Bytes(), []byte("[DEBUG] normal debug message")) {
		t.Fatalf("expected file sink to include debug message, got: %q", fileBuffer.String())
	}
}
