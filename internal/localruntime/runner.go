// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/exasol/exasol-personal/internal/localruntime/vm"
)

const defaultGuestIPv4 = "192.168.64.2"

const (
	defaultGuestSQLPort = 8563
	defaultGuestUIPort  = 8443
)

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

	if err := guest.Controller.Ensure(); err != nil {
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
		return errors.New("local runtime ports are not initialized")
	}

	sqlForwarder, err := StartLoopbackForwarder(
		ctx,
		dbPort,
		defaultGuestIPv4,
		defaultGuestSQLPort,
	)
	if err != nil {
		return err
	}
	defer sqlForwarder.Close()

	uiForwarder, err := StartLoopbackForwarder(
		ctx,
		uiPort,
		defaultGuestIPv4,
		defaultGuestUIPort,
	)
	if err != nil {
		return err
	}
	defer uiForwarder.Close()

	driver := newVMDriver()
	if err := driver.Start(ctx, guest.Machine); err != nil {
		return err
	}

	stopCtx, stopCancel := context.WithCancel(ctx)
	defer stopCancel()
	go watchStopRequest(stopCtx, guest.Controller, driver)

	if err := driver.Wait(ctx); err != nil {
		return fmt.Errorf("local runtime terminated unexpectedly: %w", err)
	}

	return nil
}

func watchStopRequest(ctx context.Context, controller Controller, driver vm.Driver) {
	stopRequestPath := controller.Paths().HostStopRequestPath
	ticker := time.NewTicker(controlPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := os.Stat(stopRequestPath)
			if err == nil {
				if stopErr := driver.Stop(ctx); stopErr != nil &&
					!errors.Is(stopErr, context.Canceled) {
					slog.Warn(
						"failed to stop local runtime VM after stop request",
						"error", stopErr,
					)
				}

				return
			}
			if !errors.Is(err, os.ErrNotExist) {
				slog.Warn(
					"failed to inspect local runtime stop-request file",
					"error", err,
				)

				return
			}
		}
	}
}
