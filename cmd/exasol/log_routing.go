// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type deploymentFileSink struct {
	mu       sync.RWMutex
	writer   io.Writer
	minLevel slog.Level
}

func (s *deploymentFileSink) Set(writer io.Writer, minLevel slog.Level) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writer = writer
	s.minLevel = minLevel
}

func (s *deploymentFileSink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writer = nil
}

func (s *deploymentFileSink) Handle(record slog.Record) error {
	s.mu.RLock()
	writer := s.writer
	minLevel := s.minLevel
	s.mu.RUnlock()

	if writer == nil {
		return nil
	}
	if record.Level < minLevel {
		return nil
	}

	_, err := writer.Write([]byte(formatFileLogRecord(record)))

	return err
}

var globalDeploymentFileSink = &deploymentFileSink{}

type routingHandler struct {
	terminalHandler slog.Handler
	fileSink        *deploymentFileSink
}

func newRoutingHandler(terminalHandler slog.Handler, fileSink *deploymentFileSink) slog.Handler {
	return &routingHandler{
		terminalHandler: terminalHandler,
		fileSink:        fileSink,
	}
}

func (h *routingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Terminal visibility follows terminal handler. File capture can still require
	// lower levels (e.g. debug) even when terminal is configured at info.
	if h.terminalHandler.Enabled(ctx, level) {
		return true
	}

	h.fileSink.mu.RLock()
	defer h.fileSink.mu.RUnlock()

	if h.fileSink.writer == nil {
		return false
	}

	return level >= h.fileSink.minLevel
}

func (h *routingHandler) Handle(ctx context.Context, record slog.Record) error {
	var terminalErr error
	var fileErr error

	if h.terminalHandler.Enabled(ctx, record.Level) {
		terminalErr = h.terminalHandler.Handle(ctx, record)
	}

	fileErr = h.fileSink.Handle(record)

	if terminalErr != nil {
		return terminalErr
	}
	if fileErr != nil {
		return fileErr
	}

	return nil
}

func (h *routingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &routingHandler{
		terminalHandler: h.terminalHandler.WithAttrs(attrs),
		fileSink:        h.fileSink,
	}
}

func (h *routingHandler) WithGroup(name string) slog.Handler {
	return &routingHandler{
		terminalHandler: h.terminalHandler.WithGroup(name),
		fileSink:        h.fileSink,
	}
}

func formatFileLogRecord(record slog.Record) string {
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	attrs := make([]string, 0)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", attr.Key, attr.Value.Any()))

		return true
	})

	attrsSuffix := ""
	if len(attrs) > 0 {
		attrsSuffix = " " + strings.Join(attrs, " ")
	}

	return fmt.Sprintf(
		"[%s] [%s] %s%s\n",
		timestamp.UTC().Format(time.RFC3339),
		levelLabel(record.Level),
		record.Message,
		attrsSuffix,
	)
}

func levelLabel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
