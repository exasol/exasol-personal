// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"log/slog"

	"github.com/exasol/exasol-personal/internal/localruntime"
)

// ErrLocalReachability is the sentinel a localReachabilityError unwraps to,
// so callers can errors.Is against it regardless of the exact message.
var ErrLocalReachability = errors.New("local runtime reachability problem")

// localReachabilityError carries the actionable, user-facing explanation for
// a network-wide local runtime reachability problem, following the same
// shape as blockedStateError: Error() is already the full message, Unwrap()
// exposes the sentinel.
type localReachabilityError struct{}

func (*localReachabilityError) Error() string { return localReachabilityMessage }
func (*localReachabilityError) Unwrap() error { return ErrLocalReachability }

const localReachabilityMessage = "could not reach the local database endpoint because the " +
	"host-to-VM network path appears blocked.\n\n" +
	"This is usually caused by macOS's \"Local Network\" permission being denied to the " +
	"terminal, editor, or agent environment that launched this command (for example iTerm2, " +
	"kitty, VS Code, or a sandboxed agent host). That permission is required even though the " +
	"reported endpoint is 127.0.0.1, because Exasol Personal forwards it from a VM over the " +
	"local network.\n\n" +
	"To fix this: open System Settings -> Privacy & Security -> Local Network, and enable " +
	"access for that application. Then retry."

// classifyLocalReachability inspects the local runtime's forwarded-port
// health and returns a localReachabilityError when every forwarded port is
// unreachable, which points at a network-wide problem rather than one
// specific to the database. It returns nil (deferring to the caller's
// original error) for non-local deployments, when the health-check itself is
// unavailable (e.g. an old runner daemon that predates health-check
// support), or when at least one port is reachable, since that means the
// problem is not network-wide.
func classifyLocalReachability(ctx context.Context, runtime *localruntime.Runtime) error {
	if !isLocalDeployment(runtime.Deployment()) {
		return nil
	}

	result, err := runtime.HealthCheck(ctx)
	if err != nil {
		slog.Debug("local health-check unavailable; skipping reachability classification",
			"error", err)

		return nil
	}

	if len(result.Ports) == 0 {
		return nil
	}

	for _, port := range result.Ports {
		switch port.State {
		case localruntime.PortStateReachable, localruntime.PortStateRefused:
			return nil
		case localruntime.PortStateBlocked, localruntime.PortStateTimeout:
			// Keep checking other ports.
		default:
			// Unknown state: don't risk misclassifying; defer to caller.
			return nil
		}
	}

	return &localReachabilityError{}
}

// diagnoseLocalFailure re-classifies a local-deployment operation failure
// as a localReachabilityError when the local runtime's forwarded ports are
// all unreachable, deferring to the original error otherwise.
func diagnoseLocalFailure(ctx context.Context, runtime *localruntime.Runtime, err error) error {
	if err == nil {
		return nil
	}

	reachabilityErr := classifyLocalReachability(ctx, runtime)
	if reachabilityErr != nil {
		return reachabilityErr
	}

	return err
}
