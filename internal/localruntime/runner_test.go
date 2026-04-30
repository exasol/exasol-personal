// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

func TestWatchStopRequest_TriggersDriverStopWhenFileAppears(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}
	controller := runtime.Controller()
	if err := controller.Ensure(); err != nil {
		t.Fatalf("expected controller Ensure to succeed, got %v", err)
	}

	driver := &fakeStopDriver{stopped: make(chan struct{})}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go watchStopRequest(ctx, controller, driver)

	// When
	if err := os.WriteFile(
		controller.Paths().HostStopRequestPath,
		[]byte("stop\n"),
		0o600,
	); err != nil {
		t.Fatalf("expected stop request to be written, got %v", err)
	}

	// Then
	select {
	case <-driver.stopped:
	case <-ctx.Done():
		t.Fatalf("expected driver.Stop to be called within timeout: %v", ctx.Err())
	}

	if got := atomic.LoadInt32(&driver.stopCalls); got != 1 {
		t.Fatalf("expected exactly one driver.Stop call, got %d", got)
	}
}

func TestWatchStopRequest_ExitsWhenContextCancelled(t *testing.T) {
	t.Parallel()

	// Given
	runtime := New(t.TempDir())
	if err := runtime.EnsureRoot(); err != nil {
		t.Fatalf("expected EnsureRoot to succeed, got %v", err)
	}
	controller := runtime.Controller()
	if err := controller.Ensure(); err != nil {
		t.Fatalf("expected controller Ensure to succeed, got %v", err)
	}

	driver := &fakeStopDriver{stopped: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchStopRequest(ctx, controller, driver)
		close(done)
	}()

	// When
	cancel()

	// Then
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected watchStopRequest to exit when context cancelled")
	}

	if got := atomic.LoadInt32(&driver.stopCalls); got != 0 {
		t.Fatalf("expected no driver.Stop calls when no stop request, got %d", got)
	}
}

type fakeStopDriver struct {
	stopCalls int32
	stopped   chan struct{}
}

func (*fakeStopDriver) Start(context.Context, vm.MachineConfig) error {
	return nil
}

func (d *fakeStopDriver) Stop(ctx context.Context) error {
	if atomic.AddInt32(&d.stopCalls, 1) == 1 {
		close(d.stopped)
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (*fakeStopDriver) Wait(context.Context) error { return nil }

func (*fakeStopDriver) Running() bool { return false }
