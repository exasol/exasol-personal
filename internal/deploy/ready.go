// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	exasolerrors "github.com/exasol/exasol-driver-go/pkg/errors"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

var (
	verifyDatabaseConnectionFn = verifyDatabaseConnection
	newExasolConnectionFn      = connect.NewExasolConnection
)

// verifyDatabaseConnection checks if the database service is accepting connections
// by attempting a connection with invalid credentials and expecting an authentication error.
func verifyDatabaseConnection(ctx context.Context, deployment config.DeploymentDir) error {
	var dbErr error
	// Suppress driver noise only for this probe (invalid creds, transient failures expected).
	probeErr := connect.WithSilencedDriverErrors(func() error {
		connectionInfo, err := config.ResolveConnectionInfo(deployment)
		if err != nil {
			return err
		}

		database, err := newExasolConnectionFn(
			deployment,
			connectionInfo,
			"invalid username",
			"invalid password",
			true,
		)
		if err != nil {
			return err
		}
		// We expect this to fail with an authentication error
		err = database.Connect(ctx)
		if err == nil {
			panic("database connection succeeded with invalid username & password")
		}
		dbErr = err

		return err
	})
	if probeErr != nil {
		// Treat connection construction errors & connect errors uniformly
		// downstream logic inspects error for SQLSTATE 08004.
		// dbErr may be the same as probeErr; use dbErr if available.
		if dbErr != nil {
			probeErr = dbErr
		}
	}

	if driverErr, ok := errors.AsType[exasolerrors.DriverErr](probeErr); ok {
		// Look for SQLSTATE error 08004. This is used for authentication failures.
		slog.Debug("received sql driver error", "error", driverErr.Error())
		if strings.Contains(driverErr.Error(), "08004") {
			return nil
		}
	}

	return probeErr
}

// WaitForDatabaseStarted polls the database connection using verifyDatabaseConnection
// until it succeeds or the timeout elapses. Provides periodic progress logs.
func WaitForDatabaseStarted(
	ctx context.Context,
	deployment config.DeploymentDir,
) error {
	return waitForDatabaseStartedWithBackoff(
		ctx,
		deployment,
		StartedInitialBackoff,
		StartedMaxBackoff,
	)
}

// localReachabilityRecheckDelay absorbs a transient container-startup race
// in which the runtime's port forwarder is briefly in an ambiguous state
// (accepts SYN but has no listener behind it yet) and returns errors that
// classifyHostPortHealth cannot map to "refused" on Windows.
const localReachabilityRecheckDelay = 3 * time.Second

func WaitForLocalDatabaseStarted(ctx context.Context, runtime localruntime.Runtime) error {
	// Fail fast on a *persistently* blocked network path instead of waiting
	// out the whole backoff window. A single classification failure is not
	// enough: container startup can transiently look blocked before the
	// forwarder settles. Re-check once after a short delay before giving up.
	if err := classifyLocalReachability(ctx, runtime); err != nil {
		slog.Debug("initial local reachability check reported network blocked; retrying",
			"error", err, "retryAfter", localReachabilityRecheckDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(localReachabilityRecheckDelay):
		}
		if err := classifyLocalReachability(ctx, runtime); err != nil {
			return err
		}
	}

	return waitForDatabaseStartedWithBackoff(
		ctx,
		runtime.Deployment(),
		LocalDatabaseStartedInitialBackoff,
		LocalDatabaseStartedMaxBackoff,
	)
}

func waitForDatabaseStartedWithBackoff(
	ctx context.Context,
	deployment config.DeploymentDir,
	initialBackoff int,
	maxBackoff int,
) error {
	return waitForDatabaseState(
		ctx,
		deployment,
		WaitParams{
			InitialBackoff: initialBackoff,
			MaxBackoff:     maxBackoff,
			ReadyMode:      true,
			LogPrefix:      "waiting for database to start",
		},
	)
}

// waitForDatabaseState consolidates the polling logic for ready & stopped states.
func waitForDatabaseState(
	ctx context.Context,
	deployment config.DeploymentDir,
	params WaitParams,
) error {
	return PollWithBackoff(ctx, func(ctx context.Context) (bool, error) {
		err := verifyDatabaseConnectionFn(ctx, deployment)
		conditionMet := (params.ReadyMode && err == nil) || (!params.ReadyMode && err != nil)

		return conditionMet, err
	}, params)
}
