// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package util

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// nolint: paralleltest
func TestRegisterSignalHandlerPanicsWhenNotInitialized(t *testing.T) {
	// Given: signal handler infrastructure has not been initialized.
	StopSignalHandler()
	t.Cleanup(func() { StopSignalHandler() })

	// When: registering a handler without calling StartSignalHandler.
	unregister, err := RegisterOnceSignalHandler(func() {})

	// Then: there must be an error
	require.ErrorIs(t, err, errNoSignalHandlerInitialization)

	defer func() {
		// Then: the unregister function must still be callable
		unregister()
	}()
}

// nolint: paralleltest
func TestEnsureOnInterruptRunsCleanupOnceWithSignal(t *testing.T) {
	// Given: cleanup handler registered with a signal channel
	StopSignalHandler()
	t.Cleanup(func() { StopSignalHandler() })

	finalHandlerCalled := make(chan os.Signal, 1)
	cleanupCalls := make(chan struct{}, 2)

	testSignalChan := make(chan os.Signal, 1)
	StartSignalHandlerWithChannel(testSignalChan, func(sig os.Signal) {
		finalHandlerCalled <- sig
	})

	cleanupOnce := EnsureOnInterrupt(func() {
		cleanupCalls <- struct{}{}
	})

	// When: a SIGINT arrives through the signal channel.
	testSignalChan <- syscall.SIGINT

	// Then: the registered handler runs exactly once and the final handler observes the signal.
	select {
	case <-cleanupCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup handler was not invoked")
	}

	select {
	case sig := <-finalHandlerCalled:
		if sig != syscall.SIGINT {
			t.Fatalf("final handler received %v, want %v", sig, syscall.SIGINT)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("final handler was not invoked")
	}

	// Then: calling the manual cleanup does not re-run the handler.
	cleanupOnce()

	select {
	case <-cleanupCalls:
		t.Fatal("cleanup handler was invoked more than once")
	default:
	}
}

// nolint: paralleltest
func TestEnsureSignalHandlersCanReregister(t *testing.T) {
	// Given: cleanup handler registered with a signal channel
	StopSignalHandler()
	t.Cleanup(func() { StopSignalHandler() })

	finalHandlerCalled := make(chan os.Signal, 1)

	testSignalChan := make(chan os.Signal, 1)
	StartSignalHandlerWithChannel(testSignalChan, func(sig os.Signal) {
		finalHandlerCalled <- sig
	})

	repeatedHandlerRunCount := 0

	var repeatingHandler func()
	repeatingHandler = func() {
		repeatedHandlerRunCount++
		_, _ = RegisterOnceSignalHandler(repeatingHandler)
	}

	_, _ = RegisterOnceSignalHandler(repeatingHandler)

	// When: a SIGINT arrives through the signal channel.
	testSignalChan <- syscall.SIGINT

	select {
	case <-finalHandlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("final handler was not invoked")
	}

	require.Equal(t, 1, repeatedHandlerRunCount)

	// Then: the repeating handler is able to re-register itself.
	testSignalChan <- syscall.SIGINT
	select {
	case <-finalHandlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("final handler was not invoked")
	}

	require.Equal(t, 2, repeatedHandlerRunCount)
}

// nolint: paralleltest
func TestEnsureResetPreservesSignalHandlers(t *testing.T) {
	// Given: cleanup handler registered with a signal channel
	StopSignalHandler()
	t.Cleanup(func() { StopSignalHandler() })

	finalHandlerCalled := make(chan os.Signal, 1)

	testSignalChan := make(chan os.Signal, 1)
	StartSignalHandlerWithChannel(testSignalChan, func(sig os.Signal) {
		finalHandlerCalled <- sig
	})

	handlerRunCount := 0

	onceHandler, err := RegisterOnceSignalHandler(func() {
		handlerRunCount++
	})

	// When: a the signal handler is reset and reinitialzied.
	StopSignalHandler()

	StartSignalHandlerWithChannel(testSignalChan, func(sig os.Signal) {
		finalHandlerCalled <- sig
	})

	// When: a SIGINT arrives through the signal channel.
	testSignalChan <- syscall.SIGINT

	select {
	case <-finalHandlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("final handler was not invoked")
	}

	// Then: the handler is run.
	require.False(t, onceHandler(), "expected onceHandler have been run")
	require.Equal(t, 1, handlerRunCount)
	require.NoError(t, err)
}
