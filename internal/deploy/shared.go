// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
)

// waitParams holds configuration for the generic waitForDatabaseState helper.
type WaitParams struct {
	InitialBackoff int
	MaxBackoff     int
	ReadyMode      bool
	LogPrefix      string
}

type BackoffCondition func(context.Context) (bool, error)

// Errors.
var (
	ErrUnknownDeploymentType      = errors.New("unknown deployment type")
	ErrNotImplemented             = errors.New("not implemented")
	ErrUnexpectedDeploymentStatus = errors.New("unexpected deployment status")
	ErrUnspportedOperation        = errors.New("unsupported operation")
)

// Default timeout configs for waiting.
const (
	StartedDefaultTimeoutInMinutes = 30
	InstanceRefreshTimeoutMinutes  = 5
	StartedDefaultTimeoutSeconds   = StartedDefaultTimeoutInMinutes * 60
	InstanceRefreshTimeoutSeconds  = InstanceRefreshTimeoutMinutes * 60
	StartedInitialBackoff          = 10
	StartedMaxBackoff              = 60
)

func Getn11Details(dir string) (*config.SSHDetails, error) {
	nodeDetails, err := config.ReadNodeDetails(dir)
	if err != nil {
		return nil, err
	}

	nodes := nodeDetails.ListNodes()

	if len(nodes) == 0 {
		return nil, ErrNoNodesFound
	}

	sshDetails, err := nodeDetails.GetSSHDetails(nodes[0])
	if err != nil {
		return nil, err
	}

	return sshDetails, nil
}

func PollWithBackoff(
	ctx context.Context,
	condition BackoffCondition,
	params WaitParams,
) error {
	backoff := params.InitialBackoff
	elapsed := 0
	var lastErr error
	var deadline time.Time
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}

	for {
		met, err := condition(ctx)
		if err != nil {
			lastErr = err
		}
		if met {
			return nil
		}

		logValues := []any{"elapsed_seconds", elapsed, "next_retry_in_seconds", backoff}
		if !deadline.IsZero() {
			remaining := int(time.Until(deadline).Seconds())
			if remaining < 0 {
				remaining = 0
			}
			logValues = append(logValues, "remaining_seconds", remaining)
		}
		slog.Info(params.LogPrefix, logValues...)

		// Sleep with backoff, handle cancellation
		select {
		case <-ctx.Done():
			if lastErr != nil {
				slog.Info(params.LogPrefix+" timeout/cancelled",
					"elapsed_seconds", elapsed, "last_error", lastErr.Error())

				return lastErr
			}

			return ctx.Err()
		case <-time.After(time.Duration(backoff) * time.Second):
		}

		elapsed += backoff
		if backoff < params.MaxBackoff {
			backoff *= 2
			if backoff > params.MaxBackoff {
				backoff = params.MaxBackoff
			}
		}
	}
}
