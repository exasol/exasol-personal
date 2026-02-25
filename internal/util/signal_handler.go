// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

// Package util provides small utility helpers.
//
// Signal handling overview
// This file implements a simple, process-wide signal dispatcher for SIGINT and
// SIGTERM.
//
// Lifecycle and usage:
//  1. Call StartSignalHandler(finalHandler) once at program startup to begin
//     listening for OS signals. finalHandler runs after all registered handlers
//     when a signal is received.
//  2. Call RegisterOnceSignalHandler(handler) to register cleanup logic. It returns
//     a deregister function that indicates whether the handler did NOT run yet.
//  3. Optionally use EnsureOnInterrupt(cleanup) with defer to guarantee cleanup
//     either on interrupt or normal return (but not twice).
//  4. Optionally use StopSignalHandler to return the program to default signal
//     handling behaviour.
//
// Concurrency model:
//   - Registration and deregistration take a mutex.
//   - On first received signal, all current once-handlers are moved to a local slice,
//     the mutex is unlock, the once-handlers are invoked.
//   - Both once-handlers and finalHandler are invoked outside the lock to avoid
//     deadlocks and allow reentrancy.
//   - Subsequent signals are ignored (single-shot semantics).
package util

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var errNoSignalHandlerInitialization = errors.New("signal handler was not initialized")

var signalHandler = struct {
	mtx             sync.Mutex
	nextHandlerId   int
	onceHandlers    map[int]func()
	finalHandler    func(os.Signal)
	cancel          func()
	workerWaitGroup sync.WaitGroup
}{
	nextHandlerId: 0,
	onceHandlers:  make(map[int]func()),
}

// RegisterOnceSignalHandler adds a SIGINT or SIGTERM handler that runs once.
// returns a function to remove the handler.
// The returned deregister function returns true if the handler was not run.
// StartSignalHandler should be called first.
func RegisterOnceSignalHandler(handler func()) (func() bool, error) {
	signalHandler.mtx.Lock()
	defer signalHandler.mtx.Unlock()

	if signalHandler.finalHandler == nil {
		return func() bool { return true }, errNoSignalHandlerInitialization
	}

	handlerId := signalHandler.nextHandlerId
	signalHandler.nextHandlerId++

	signalHandler.onceHandlers[handlerId] = handler

	return func() bool {
		signalHandler.mtx.Lock()
		defer signalHandler.mtx.Unlock()

		if _, ok := signalHandler.onceHandlers[handlerId]; ok {
			delete(signalHandler.onceHandlers, handlerId)
			return true
		}

		return false
	}, nil
}

func handleSignal(sig os.Signal) {
	signalHandler.mtx.Lock()

	onceHandlers := signalHandler.onceHandlers
	signalHandler.onceHandlers = make(map[int]func())

	finalHandler := signalHandler.finalHandler

	signalHandler.mtx.Unlock()

	for _, handler := range onceHandlers {
		handler()
	}

	if finalHandler != nil {
		finalHandler(sig)
	}
}

// StartSignalHandler wires OS signal delivery and sets the finalHandler to be
// called after all registered handlers when a SIGINT or SIGTERM is received.
//
// Constraints:
// - Must be called exactly once; subsequent calls without calling ResetSignalHandler panic.
// - finalHandler may be nil; in that case only registered handlers run.
func StartSignalHandler(finalHandler func(os.Signal)) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	StartSignalHandlerWithChannel(sigs, finalHandler)
}

func StartSignalHandlerWithChannel(sigs chan os.Signal, finalHandler func(os.Signal)) {
	signalHandler.mtx.Lock()
	defer signalHandler.mtx.Unlock()

	if signalHandler.cancel != nil {
		panic("signal handler initialized multiple times")
	}

	signalHandler.finalHandler = finalHandler

	if signalHandler.onceHandlers == nil {
		signalHandler.onceHandlers = make(map[int]func())
	}

	signalHandler.workerWaitGroup.Add(1)

	ctx, cancel := context.WithCancel(context.Background())
	signalHandler.cancel = cancel

	go func() {
		defer signalHandler.workerWaitGroup.Done()
		for {
			select {
			case sig := <-sigs:
				handleSignal(sig)

			case <-ctx.Done():
				signal.Stop(sigs)
				return
			}
		}
	}()
}

// StopSignalHandler reverses StartSignalHandler, causing the program to revert to the default
// signal handling behaviour
// Constraints:
// - If StartSignalHandler was not previously run, this function has not effect.
// - StartSignalHandler can be run after StopSignalHandler has been run.
// - This does not deregister once-handlers. They will remain after re-initializing.
func StopSignalHandler() {
	signalHandler.mtx.Lock()

	if signalHandler.cancel != nil {
		signalHandler.cancel()
	}

	signalHandler.finalHandler = nil

	signalHandler.mtx.Unlock()

	signalHandler.workerWaitGroup.Wait()

	signalHandler.mtx.Lock()
	defer signalHandler.mtx.Unlock()

	signalHandler.cancel = nil
}

// EnsureOnInterrupt runs a cleanup function once if a SIGINT or SIGTERM occurs, or
// if the returned function is called. Expected usage with defer.
func EnsureOnInterrupt(cleanup func()) func() {
	unregister, err := RegisterOnceSignalHandler(cleanup)
	if err != nil {
		return cleanup
	}

	return func() {
		if unregister() {
			cleanup()
		}
	}
}
