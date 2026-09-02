// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"testing"

	"github.com/exasol/exasol-driver-go/pkg/logger"
)

type testDriverLogger struct{}

func (*testDriverLogger) Print(...any)          {}
func (*testDriverLogger) Printf(string, ...any) {}

//nolint:paralleltest // The test temporarily replaces the process-wide driver logger.
func TestWithSilencedDriverErrorsRestoresLoggerAfterOverlappingCalls(t *testing.T) {
	// Given
	previous := logger.ErrorLogger
	original := &testDriverLogger{}
	if err := logger.SetLogger(original); err != nil {
		t.Fatalf("set test logger: %v", err)
	}
	t.Cleanup(func() { _ = logger.SetLogger(previous) })

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	// When
	go func() {
		firstDone <- WithSilencedDriverErrors(func() error {
			close(firstStarted)
			<-releaseFirst

			return nil
		})
	}()
	<-firstStarted
	go func() {
		secondDone <- WithSilencedDriverErrors(func() error {
			close(secondStarted)
			<-releaseSecond

			return nil
		})
	}()
	<-secondStarted
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first callback failed: %v", err)
	}

	// Then the logger remains silenced until the overlapping call finishes.
	if logger.ErrorLogger == original {
		t.Fatal("expected driver errors to remain silenced")
	}
	close(releaseSecond)
	if err := <-secondDone; err != nil {
		t.Fatalf("second callback failed: %v", err)
	}
	if logger.ErrorLogger != original {
		t.Fatal("expected original driver logger to be restored")
	}
}
