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

var (
	loggerMutex               sync.Mutex
	silencedDriverErrorUsers  int
	originalDriverErrorLogger logger.Logger
)

// WithSilencedDriverErrors runs fn with driver errors suppressed.
func WithSilencedDriverErrors(callback func() error) error { //nolint:revive
	loggerMutex.Lock()
	if silencedDriverErrorUsers == 0 {
		originalDriverErrorLogger = logger.ErrorLogger
		_ = logger.SetLogger(discardLogger{})
	}
	silencedDriverErrorUsers++
	loggerMutex.Unlock()

	defer func() {
		loggerMutex.Lock()
		silencedDriverErrorUsers--
		if silencedDriverErrorUsers == 0 {
			_ = logger.SetLogger(originalDriverErrorLogger)
			originalDriverErrorLogger = nil
		}
		loggerMutex.Unlock()
	}()

	return callback()
}
