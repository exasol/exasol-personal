// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

// withFakeVerifyDatabaseConnection swaps the package-level
// verifyDatabaseConnectionFn seam so WaitForDatabaseStarted/
// WaitForLocalDatabaseStarted can be driven without a real database.
func withFakeVerifyDatabaseConnection(
	t *testing.T,
	fn func(context.Context, config.DeploymentDir) error,
) {
	t.Helper()

	original := verifyDatabaseConnectionFn
	verifyDatabaseConnectionFn = fn
	t.Cleanup(func() {
		verifyDatabaseConnectionFn = original
	})
}

//nolint:paralleltest // mutates the package-level verifyDatabaseConnectionFn seam.
func TestWaitForDatabaseStarted_SucceedsImmediatelyWhenConnectionVerified(t *testing.T) {
	withFakeVerifyDatabaseConnection(t, func(context.Context, config.DeploymentDir) error {
		return nil
	})

	deployment := config.NewDeploymentDir(t.TempDir())

	if err := WaitForDatabaseStarted(context.Background(), deployment); err != nil {
		t.Fatalf("expected an immediately-ready database to succeed, got %v", err)
	}
}

//nolint:paralleltest // mutates the package-level verifyDatabaseConnectionFn seam.
func TestWaitForDatabaseStarted_PropagatesLastErrorWhenContextEnds(t *testing.T) {
	probeErr := errors.New("connection refused")
	withFakeVerifyDatabaseConnection(t, func(context.Context, config.DeploymentDir) error {
		return probeErr
	})

	deployment := config.NewDeploymentDir(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := WaitForDatabaseStarted(ctx, deployment)
	if !errors.Is(err, probeErr) {
		t.Fatalf("expected the last probe error to surface, got %v", err)
	}
}

//nolint:paralleltest // mutates the package-level verifyDatabaseConnectionFn seam.
func TestWaitForLocalDatabaseStarted_NonLocalDeploymentStillPolls(t *testing.T) {
	withFakeVerifyDatabaseConnection(t, func(context.Context, config.DeploymentDir) error {
		return nil
	})

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := os.MkdirAll(deployment.InfrastructureDir(), 0o700); err != nil {
		t.Fatalf("create infrastructure dir failed: %v", err)
	}
	writeTestFile(t, deployment.InfrastructureManifestPath(), `
name: Test Infrastructure
description: test infrastructure
backend: tofu
`)

	runtime := localruntime.New(deployment, nil)

	if err := WaitForLocalDatabaseStarted(context.Background(), runtime); err != nil {
		t.Fatalf("expected a non-local deployment to skip reachability and poll, got %v", err)
	}
}

// nolint: paralleltest // avoids concurrent extract+exec of the fake runner (ETXTBSY flakes).
func TestWaitForLocalDatabaseStarted_FailsFastOnBlockedReachability(t *testing.T) {
	skipOnWindows(t)

	deployment := newLocalTestDeployment(t)
	ensureLocalRuntimeWorkDir(t, deployment)
	blockedJSON := allPortsBlockedHealthJSON
	manager := writeFakeCombinedRunner(t, "", blockedJSON)
	runtime := localruntime.New(deployment, manager)

	err := WaitForLocalDatabaseStarted(context.Background(), runtime)
	if !errors.Is(err, ErrLocalReachability) {
		t.Fatalf("expected a reachability error before any DB polling, got %v", err)
	}
}
