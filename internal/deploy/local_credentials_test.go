// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	exasolerrors "github.com/exasol/exasol-driver-go/pkg/errors"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect/exasol"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

func resolveTestInitialPassword(t *testing.T, deployment config.DeploymentDir) string {
	t.Helper()
	password, err := resolveLocalInitialPassword(deployment)
	if err != nil {
		t.Fatalf("expected initial password to resolve, got %v", err)
	}

	return password
}

func assertStoredPassword(t *testing.T, deployment config.DeploymentDir, expected string) {
	t.Helper()
	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		t.Fatalf("expected secrets to be readable, got %v", err)
	}
	if secrets.DbPassword != expected {
		t.Fatalf("expected stored password %q, got %q", expected, secrets.DbPassword)
	}
}

func TestResolveLocalInitialPassword_GeneratesAndRecordsPassword(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)

	// When
	password := resolveTestInitialPassword(t, deployment)

	// Then
	if len(password) != databasePasswordLength || password == localDBPassword {
		t.Fatalf("expected a generated password, got %q", password)
	}
	assertStoredPassword(t, deployment, password)
	generated, err := localDatabasePasswordGenerated(deployment)
	if err != nil || !generated {
		t.Fatalf("expected the generated credential to be recorded, got %t, %v", generated, err)
	}
}

func TestResolveLocalInitialPassword_ReusesStoredGeneratedPassword(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	first := resolveTestInitialPassword(t, deployment)

	// When
	second := resolveTestInitialPassword(t, deployment)

	// Then
	if second != first {
		t.Fatalf("expected a retried initialization to reuse %q, got %q", first, second)
	}
	assertStoredPassword(t, deployment, first)
}

func TestResolveLocalInitialPassword_ReplacesUngeneratedPassword(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	stored := &config.Secrets{DbPassword: localDBPassword, AdminUiPassword: "admin"}
	if err := config.WriteSecrets(deployment.Root(), stored); err != nil {
		t.Fatalf("failed to write legacy secrets: %v", err)
	}

	// When
	password := resolveTestInitialPassword(t, deployment)

	// Then
	if password == localDBPassword {
		t.Fatal("expected the legacy default to be replaced for a fresh database")
	}
	secrets, err := config.ReadSecrets(deployment)
	if err != nil || secrets.DbPassword != password || secrets.AdminUiPassword != "admin" {
		t.Fatalf("expected only the database password to change, got %#v, %v", secrets, err)
	}
}

func TestResolveLocalInitialPassword_GeneratesWhenRecordedPasswordIsMissing(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	if err := markLocalDatabasePasswordGenerated(deployment); err != nil {
		t.Fatalf("failed to mark generated credential: %v", err)
	}

	// When
	password := resolveTestInitialPassword(t, deployment)

	// Then
	if password == "" {
		t.Fatal("expected a password for a fresh database")
	}
	assertStoredPassword(t, deployment, password)
}

func TestClassifyLocalCredentialCheck(t *testing.T) {
	t.Parallel()

	connectionFailure := errors.New("connection refused")
	for _, test := range []struct {
		name       string
		err        error
		expectNil  bool
		rejected   bool
		underlying error
	}{
		{name: "accepted", err: nil, expectNil: true},
		{
			name:     "authentication rejected",
			err:      exasolerrors.NewSqlErr("08004", "authentication failed"),
			rejected: true,
		},
		{name: "other failure", err: connectionFailure, underlying: connectionFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// When
			err := classifyLocalCredentialCheck(test.err)

			// Then
			if test.expectNil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}

				return
			}
			if errors.Is(err, errLocalStoredCredentialRejected) != test.rejected {
				t.Fatalf("unexpected rejection classification for %v", err)
			}
			if test.underlying != nil && !errors.Is(err, test.underlying) {
				t.Fatalf("expected underlying error to be kept, got %v", err)
			}
		})
	}
}

func withFakeStoredCredentialVerification(
	t *testing.T,
	verify func(context.Context, config.DeploymentDir) error,
) *int {
	t.Helper()
	calls := 0
	original := verifyLocalStoredCredentialFn
	verifyLocalStoredCredentialFn = func(
		ctx context.Context,
		deployment config.DeploymentDir,
	) error {
		calls++
		return verify(ctx, deployment)
	}
	t.Cleanup(func() {
		verifyLocalStoredCredentialFn = original
	})
	withFakeDatabaseConnectionVerification(t, func(context.Context, config.DeploymentDir) error {
		return nil
	})

	return &calls
}

//nolint:paralleltest // Mutates package-level verification functions.
func TestStartPreparedLocalRuntime_VerifiesStoredCredentialUntilLoginSucceeds(t *testing.T) {
	for _, test := range []struct {
		name             string
		existingMarker   *localCredentialMarker
		initializeFresh  bool
		verifyErr        error
		expectedCalls    int
		expectedStops    int
		expectedVerified bool
	}{
		{
			name:             "first initialization accepted",
			initializeFresh:  true,
			expectedCalls:    1,
			expectedVerified: true,
		},
		{
			name:            "first initialization rejected",
			initializeFresh: true,
			verifyErr:       errLocalStoredCredentialRejected,
			expectedCalls:   1,
			expectedStops:   1,
		},
		{
			name:           "retry after a rejected login",
			existingMarker: &localCredentialMarker{DbPasswordGenerated: true},
			verifyErr:      errLocalStoredCredentialRejected,
			expectedCalls:  1,
			expectedStops:  1,
		},
		{
			name:             "retry after an interrupted check",
			existingMarker:   &localCredentialMarker{DbPasswordGenerated: true},
			expectedCalls:    1,
			expectedVerified: true,
		},
		{
			name: "already verified",
			existingMarker: &localCredentialMarker{
				DbPasswordGenerated: true, DbPasswordVerified: true,
			},
			expectedVerified: true,
		},
		{name: "existing database", expectedCalls: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Given
			calls := withFakeStoredCredentialVerification(t,
				func(context.Context, config.DeploymentDir) error { return test.verifyErr })
			deployment := newTestDeploymentWithState(t)
			givenStoredCredential(t, deployment, test.existingMarker)
			runtime := &endpointRuntimeStub{
				deployment:      deployment,
				endpoint:        &localruntime.RuntimeEndpoint{DBPort: localTestDatabasePort},
				initializeFresh: test.initializeFresh,
			}

			// When
			err := startPreparedLocalRuntime(
				context.Background(), runtime, localRuntimeConfig{}, 0, nil, nil,
			)

			// Then
			assertErrorIs(t, err, test.verifyErr)
			if *calls != test.expectedCalls || runtime.stopCalls != test.expectedStops {
				t.Fatalf(
					"expected %d verifications and %d stops, got %d and %d",
					test.expectedCalls, test.expectedStops, *calls, runtime.stopCalls,
				)
			}
			assertCredentialVerified(t, deployment, test.expectedVerified)
		})
	}
}

func givenStoredCredential(
	t *testing.T,
	deployment config.DeploymentDir,
	marker *localCredentialMarker,
) {
	t.Helper()
	if marker == nil {
		return
	}
	if err := writeLocalCredentialMarker(deployment, *marker); err != nil {
		t.Fatalf("failed to write credential marker: %v", err)
	}
	stored := &config.Secrets{DbPassword: "stored-password"}
	if err := config.WriteSecrets(deployment.Root(), stored); err != nil {
		t.Fatalf("failed to write stored secrets: %v", err)
	}
}

func assertErrorIs(t *testing.T, err, expected error) {
	t.Helper()
	if !errors.Is(err, expected) || (expected == nil && err != nil) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

func assertCredentialVerified(t *testing.T, deployment config.DeploymentDir, expected bool) {
	t.Helper()
	marker, err := readLocalCredentialMarker(deployment)
	if err != nil || marker.DbPasswordVerified != expected {
		t.Fatalf("expected verified=%t, got %#v, %v", expected, marker, err)
	}
}

func TestWriteLocalCredentialMarker_ReplacesMarkerAtomically(t *testing.T) {
	t.Parallel()

	// Given
	deployment := newTestDeploymentWithState(t)
	if err := markLocalDatabasePasswordGenerated(deployment); err != nil {
		t.Fatalf("failed to mark generated credential: %v", err)
	}

	// When
	err := markLocalDatabasePasswordVerified(deployment)
	// Then
	if err != nil {
		t.Fatalf("expected the marker to be replaced, got %v", err)
	}
	marker, err := readLocalCredentialMarker(deployment)
	if err != nil || !marker.DbPasswordGenerated || !marker.DbPasswordVerified {
		t.Fatalf("expected a generated and verified marker, got %#v, %v", marker, err)
	}
	entries, err := os.ReadDir(filepath.Dir(localruntime.CredentialMarkerPath(deployment)))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected only the marker file to remain, got %v, %v", entries, err)
	}
}

type deadlineRecordingDatabase struct {
	generaltypes.Databaser

	hadDeadline bool
}

func (database *deadlineRecordingDatabase) Connect(ctx context.Context) error {
	_, database.hadDeadline = ctx.Deadline()

	return nil
}

func (*deadlineRecordingDatabase) Close() error {
	return nil
}

//nolint:paralleltest // Mutates package-level newExasolConnectionFn.
func TestVerifyLocalStoredCredential_BoundsTheLogin(t *testing.T) {
	// Given
	deployment := newTestDeploymentWithState(t)
	endpoint := &localruntime.VMRuntimeEndpoint{
		RuntimeEndpoint: localruntime.RuntimeEndpoint{DBPort: localTestDatabasePort},
	}
	if err := writeLocalDeploymentArtifacts(deployment, endpoint); err != nil {
		t.Fatalf("failed to write deployment artifacts: %v", err)
	}
	database := &deadlineRecordingDatabase{}
	original := newExasolConnectionFn
	newExasolConnectionFn = func(
		config.DeploymentDir, *config.ConnectionInfo, string, string, bool, ...exasol.OptFn,
	) (generaltypes.Databaser, error) {
		return database, nil
	}
	t.Cleanup(func() { newExasolConnectionFn = original })

	// When
	err := verifyLocalStoredCredential(context.Background(), deployment)
	// Then
	if err != nil {
		t.Fatalf("expected verification to succeed, got %v", err)
	}
	if !database.hadDeadline {
		t.Fatal("expected the login to run under a deadline")
	}
}
