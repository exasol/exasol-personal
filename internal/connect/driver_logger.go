// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"sync"

	"github.com/exasol/exasol-driver-go/pkg/logger"
)

// discardLogger suppresses all driver error output.
type discardLogger struct{}

func (discardLogger) Print(_ ...any)            { /* suppressed */ }
func (discardLogger) Printf(_ string, _ ...any) { /* suppressed */ }

var loggerMutex sync.Mutex

// withDriverErrorLogger runs fn with the driver ErrorLogger temporarily replaced.
// It restores the original logger afterwards. Not safe for concurrent mixed usage;
// guarded by a mutex.
func withDriverErrorLogger(temp logger.Logger, callback func() error) error {
	loggerMutex.Lock()
	old := logger.ErrorLogger
	_ = logger.SetLogger(temp)
	loggerMutex.Unlock()

	defer func() {
		loggerMutex.Lock()
		_ = logger.SetLogger(old)
		loggerMutex.Unlock()
	}()

	return callback()
}

// withSilencedDriverErrors runs fn with driver errors suppressed.
// Exported for readiness probes & tests.
func WithSilencedDriverErrors(fn func() error) error { //nolint:revive
	return withDriverErrorLogger(discardLogger{}, fn)
}
