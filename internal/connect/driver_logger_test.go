// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"bytes"
	"errors"
	"log"
	"testing"

	"github.com/exasol/exasol-driver-go/pkg/logger"
)

// nolint: paralleltest // these tests serialize access to the driver's shared ErrorLogger global.
func TestWithSilencedDriverErrors_SuppressesOutputDuringCallback(t *testing.T) {
	var buffer bytes.Buffer
	original := log.New(&buffer, "prefix: ", 0)
	restore := setDriverErrorLoggerForTest(t, original)
	defer restore()

	err := WithSilencedDriverErrors(func() error {
		logger.ErrorLogger.Print("should not appear")
		logger.ErrorLogger.Printf("should also not appear %s", "arg")

		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if buffer.Len() != 0 {
		t.Fatalf("expected driver errors to be suppressed, got %q", buffer.String())
	}
}

// nolint: paralleltest // these tests serialize access to the driver's shared ErrorLogger global.
func TestWithSilencedDriverErrors_RestoresOriginalLoggerAfterCallback(t *testing.T) {
	var buffer bytes.Buffer
	original := log.New(&buffer, "prefix: ", 0)
	restore := setDriverErrorLoggerForTest(t, original)
	defer restore()

	if err := WithSilencedDriverErrors(func() error { return nil }); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	logger.ErrorLogger.Print("visible again")
	if buffer.String() != "prefix: visible again\n" {
		t.Fatalf("expected the original logger to be restored, got %q", buffer.String())
	}
}

// nolint: paralleltest // these tests serialize access to the driver's shared ErrorLogger global.
func TestWithSilencedDriverErrors_RestoresOriginalLoggerEvenOnCallbackError(t *testing.T) {
	var buffer bytes.Buffer
	original := log.New(&buffer, "prefix: ", 0)
	restore := setDriverErrorLoggerForTest(t, original)
	defer restore()

	callbackErr := errors.New("callback failed")
	err := WithSilencedDriverErrors(func() error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("expected the callback error to propagate, got %v", err)
	}

	logger.ErrorLogger.Print("visible again")
	if buffer.String() != "prefix: visible again\n" {
		t.Fatalf("expected the original logger to be restored, got %q", buffer.String())
	}
}

// nolint: paralleltest // these tests serialize access to the driver's shared ErrorLogger global.
func TestWithDriverErrorLogger_UsesGivenLoggerDuringCallback(t *testing.T) {
	var buffer bytes.Buffer
	temp := log.New(&buffer, "temp: ", 0)
	restore := setDriverErrorLoggerForTest(t, log.New(&bytes.Buffer{}, "unused: ", 0))
	defer restore()

	err := withDriverErrorLogger(temp, func() error {
		logger.ErrorLogger.Print("hello")

		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if buffer.String() != "temp: hello\n" {
		t.Fatalf("expected the temporary logger to receive the message, got %q", buffer.String())
	}
}

// setDriverErrorLoggerForTest installs logger as the driver's ErrorLogger and
// returns a function that restores whatever was active before. The driver
// exposes ErrorLogger as a bare package variable with no test seam, so tests
// touching it must serialize instead of running in parallel with each other.
func setDriverErrorLoggerForTest(t *testing.T, temp logger.Logger) func() {
	t.Helper()

	previous := logger.ErrorLogger
	if err := logger.SetLogger(temp); err != nil {
		t.Fatalf("failed to set driver error logger: %v", err)
	}

	return func() {
		_ = logger.SetLogger(previous)
	}
}
