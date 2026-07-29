// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

//nolint:paralleltest // The test replaces the package-level database probe.
func TestWaitForDatabaseStateRetriesUntilRequestedState(t *testing.T) {
	original := verifyDatabaseConnectionFn
	t.Cleanup(func() { verifyDatabaseConnectionFn = original })

	tests := []struct {
		name      string
		readyMode bool
		results   []error
	}{
		{
			name:      "database becomes ready",
			readyMode: true,
			results:   []error{errors.New("starting"), errors.New("starting"), nil},
		},
		{
			name:      "database becomes stopped",
			readyMode: false,
			results:   []error{nil, nil, errors.New("connection refused")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			verifyDatabaseConnectionFn = func(
				context.Context,
				config.DeploymentDir,
			) error {
				result := test.results[calls]
				calls++

				return result
			}

			err := waitForDatabaseState(
				context.Background(),
				config.NewDeploymentDir(t.TempDir()),
				WaitParams{
					InitialBackoff: 0,
					MaxBackoff:     0,
					ReadyMode:      test.readyMode,
					LogPrefix:      "test database state",
				},
			)
			if err != nil {
				t.Fatalf("waitForDatabaseState() error = %v", err)
			}
			if calls != len(test.results) {
				t.Fatalf("database probe calls = %d, want %d", calls, len(test.results))
			}
		})
	}
}

//nolint:paralleltest // The test replaces the package-level database probe.
func TestWaitForDatabaseStateReturnsLastProbeErrorOnCancellation(t *testing.T) {
	original := verifyDatabaseConnectionFn
	t.Cleanup(func() { verifyDatabaseConnectionFn = original })

	probeErr := errors.New("database is still starting")
	ctx, cancel := context.WithCancel(context.Background())
	verifyDatabaseConnectionFn = func(context.Context, config.DeploymentDir) error {
		cancel()

		return probeErr
	}

	err := waitForDatabaseState(
		ctx,
		config.NewDeploymentDir(t.TempDir()),
		WaitParams{
			InitialBackoff: 1,
			MaxBackoff:     1,
			ReadyMode:      true,
			LogPrefix:      "test database state",
		},
	)
	if !errors.Is(err, probeErr) {
		t.Fatalf("waitForDatabaseState() error = %v, want %v", err, probeErr)
	}
}
