// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

const defaultGuestIPv4 = "192.168.64.2"

var newVMDriver = vm.New

func (r *Runtime) Run(ctx context.Context) error {
	if err := r.EnsureRoot(); err != nil {
		return err
	}
	if _, err := r.EnsurePayloadSelected(ctx); err != nil {
		return err
	}
	if err := r.ResetControlState(); err != nil {
		return err
	}

	guest, err := r.PrepareGuest(ctx)
	if err != nil {
		return err
	}

	state, err := r.LoadState()
	if err != nil {
		return err
	}

	if err := r.WriteRunnerPID(os.Getpid()); err != nil {
		return err
	}
	defer func() {
		if err := r.CleanupTransientState(); err != nil {
			slog.Warn("failed to clean up local runtime state", "error", err)
		}
	}()

	dbPort := state.Ports["db"]
	uiPort := state.Ports["ui"]
	if dbPort <= 0 || uiPort <= 0 {
		return fmt.Errorf("local runtime ports are not initialized")
	}

	sqlForwarder, err := StartLoopbackForwarder(dbPort, defaultGuestIPv4, 8563)
	if err != nil {
		return err
	}
	defer sqlForwarder.Close()

	uiForwarder, err := StartLoopbackForwarder(uiPort, defaultGuestIPv4, 8443)
	if err != nil {
		return err
	}
	defer uiForwarder.Close()

	driver := newVMDriver()
	if err := driver.Start(ctx, guest.Machine); err != nil {
		return err
	}

	if err := driver.Wait(ctx); err != nil {
		return fmt.Errorf("local runtime terminated unexpectedly: %w", err)
	}

	return nil
}
