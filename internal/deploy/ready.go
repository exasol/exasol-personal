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

func WaitForLocalDatabaseStarted(ctx context.Context, runtime localruntime.Runtime) error {
	waitCtx, cancel := localReadinessPollingContext(ctx)
	defer cancel()

	err := waitForDatabaseStartedWithBackoff(
		waitCtx,
		runtime.Deployment(),
		LocalDatabaseStartedInitialBackoff,
		LocalDatabaseStartedMaxBackoff,
	)
	if err != nil {
		diagnosisCtx, cancel := context.WithTimeout(ctx, localReadinessDiagnosisWindow)
		defer cancel()

		return diagnoseLocalFailure(diagnosisCtx, runtime, err)
	}

	return nil
}

const (
	localReadinessDiagnosisWindow        = 2 * time.Second
	localReadinessDiagnosisWindowDivisor = 2
)

func localReadinessPollingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx, func() {}
	}

	remaining := time.Until(deadline)
	reserved := min(localReadinessDiagnosisWindow, remaining/localReadinessDiagnosisWindowDivisor)

	return context.WithTimeout(ctx, remaining-reserved)
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
