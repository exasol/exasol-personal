// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

func withFakeDatabaseConnectionVerification(
	t *testing.T,
	verify func(context.Context, config.DeploymentDir) error,
) {
	t.Helper()

	original := verifyDatabaseConnectionFn
	verifyDatabaseConnectionFn = verify
	t.Cleanup(func() {
		verifyDatabaseConnectionFn = original
	})
}

//nolint:paralleltest // Mutates package-level verifyDatabaseConnectionFn.
func TestWaitForDatabaseStartedKeepsCloudReadinessPolicy(t *testing.T) {
	// Given
	withFakeDatabaseConnectionVerification(t, func(context.Context, config.DeploymentDir) error {
		return nil
	})

	// When
	err := WaitForDatabaseStarted(context.Background(), config.NewDeploymentDir(t.TempDir()))
	// Then
	if err != nil {
		t.Fatalf("expected cloud readiness to succeed immediately, got %v", err)
	}
	if StartedInitialBackoff != 10 || StartedMaxBackoff != 60 ||
		StartedDefaultTimeoutSeconds != 30*60 {
		t.Fatalf(
			"cloud readiness policy changed: initial=%d max=%d timeout=%d",
			StartedInitialBackoff,
			StartedMaxBackoff,
			StartedDefaultTimeoutSeconds,
		)
	}
}

func TestBoundedLocalDatabaseStartedTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{name: "default", requested: 0, want: 30},
		{name: "shorter caller timeout", requested: 15, want: 15},
		{name: "longer caller timeout", requested: 60, want: 30},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := boundedLocalDatabaseStartedTimeout(test.requested)

			// Then
			if got != test.want {
				t.Fatalf("expected timeout %d, got %d", test.want, got)
			}
		})
	}
}

//nolint:paralleltest // Mutates package-level verifyDatabaseConnectionFn.
func TestWaitForLocalDatabaseStartedPreservesConnectionFailureWithGuidance(t *testing.T) {
	// Given
	connectionErr := errors.New("database connection refused")
	withFakeDatabaseConnectionVerification(t, func(context.Context, config.DeploymentDir) error {
		return connectionErr
	})
	runtime := &endpointRuntimeStub{
		deployment: newLocalTestDeployment(t),
		healthResult: &localruntime.HealthCheckResult{Ports: map[string]localruntime.PortHealth{
			"db": {State: localruntime.PortStateBlocked},
		}},
		honorContext: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// When
	err := WaitForLocalDatabaseStarted(ctx, runtime)

	// Then
	if !errors.Is(err, connectionErr) {
		t.Fatalf("expected the last connection failure, got %v", err)
	}
	if !errors.Is(err, ErrLocalReachability) {
		t.Fatalf("expected local reachability guidance, got %v", err)
	}
}

//nolint:paralleltest // Mutates package-level verifyDatabaseConnectionFn.
func TestWaitForLocalDatabaseStartedSucceedsWithoutExtraDelay(t *testing.T) {
	// Given
	withFakeDatabaseConnectionVerification(t, func(context.Context, config.DeploymentDir) error {
		return nil
	})
	runtime := &endpointRuntimeStub{deployment: newLocalTestDeployment(t)}

	// When
	err := WaitForLocalDatabaseStarted(context.Background(), runtime)
	// Then
	if err != nil {
		t.Fatalf("expected an immediately ready local database to succeed, got %v", err)
	}
	if LocalDatabaseStartedInitialBackoff != 1 || LocalDatabaseStartedMaxBackoff != 2 ||
		LocalDatabaseStartedDefaultTimeoutSeconds != 30 {
		t.Fatalf(
			"unexpected local readiness policy: initial=%d max=%d timeout=%d",
			LocalDatabaseStartedInitialBackoff,
			LocalDatabaseStartedMaxBackoff,
			LocalDatabaseStartedDefaultTimeoutSeconds,
		)
	}
}
