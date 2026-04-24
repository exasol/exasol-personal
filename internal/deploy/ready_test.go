// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

type stubDatabase struct {
	connectErr error
	execErr    error
	closeErr   error
	closeFn    func()
	execFn     func(string)
}

func (s stubDatabase) Connect(context.Context) error { return s.connectErr }
func (s stubDatabase) Exec(_ context.Context, query string) (generaltypes.QueryResulter, error) {
	if s.execFn != nil {
		s.execFn(query)
	}

	return nil, s.execErr
}
func (s stubDatabase) Close() error {
	if s.closeFn != nil {
		s.closeFn()
	}

	return s.closeErr
}

func TestVerifyDatabaseConnection_UsesRealCredentialsForLocalBackend(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword: localDefaultDatabasePassword,
	}); err != nil {
		t.Fatalf("failed to write local secrets: %v", err)
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend: config.DeploymentBackendLocal,
		Connection: &config.DeploymentConnection{
			Host:                       "127.0.0.1",
			DisplayHost:                "localhost",
			DBPort:                     8563,
			UIPort:                     8443,
			Username:                   localDefaultDatabaseUser,
			InsecureSkipCertValidation: true,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	originalNewExasolProbeConnection := newExasolProbeConnectionFn
	t.Cleanup(func() {
		newExasolProbeConnectionFn = originalNewExasolProbeConnection
	})

	called := false
	executedQuery := ""
	newExasolProbeConnectionFn = func(
		gotDeployment config.DeploymentDir,
		connectionInfo *config.ConnectionInfo,
		username string,
		password string,
		insecureSkipCertValidation bool,
	) (generaltypes.Databaser, error) {
		called = true
		if gotDeployment.Root() != deployment.Root() {
			t.Fatalf("expected deployment %q, got %q", deployment.Root(), gotDeployment.Root())
		}
		if connectionInfo == nil || connectionInfo.DBPort != 8563 {
			t.Fatalf("unexpected connection info: %#v", connectionInfo)
		}
		if username != localDefaultDatabaseUser {
			t.Fatalf("expected username %q, got %q", localDefaultDatabaseUser, username)
		}
		if password != "" {
			t.Fatalf("expected launcher to let connection helper load password, got %q", password)
		}
		if !insecureSkipCertValidation {
			t.Fatal("expected insecure local readiness connection")
		}

		return stubDatabase{
			execFn: func(query string) {
				executedQuery = query
			},
		}, nil
	}

	if err := verifyDatabaseConnection(context.Background(), deployment); err != nil {
		t.Fatalf("expected local readiness probe to succeed, got %v", err)
	}
	if !called {
		t.Fatal("expected local readiness probe to create a database connection")
	}
	if executedQuery != "SELECT 1" {
		t.Fatalf("expected readiness query %q, got %q", "SELECT 1", executedQuery)
	}
}

func TestVerifyDatabaseConnection_LocalBackendDoesNotCloseBeforeConnectSucceeds(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword: localDefaultDatabasePassword,
	}); err != nil {
		t.Fatalf("failed to write local secrets: %v", err)
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend: config.DeploymentBackendLocal,
		Connection: &config.DeploymentConnection{
			Host:                       "127.0.0.1",
			DisplayHost:                "localhost",
			DBPort:                     8563,
			UIPort:                     8443,
			Username:                   localDefaultDatabaseUser,
			InsecureSkipCertValidation: true,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	originalNewExasolProbeConnection := newExasolProbeConnectionFn
	t.Cleanup(func() {
		newExasolProbeConnectionFn = originalNewExasolProbeConnection
	})

	connectErr := errors.New("connection refused")
	closeCalled := false
	newExasolProbeConnectionFn = func(
		config.DeploymentDir,
		*config.ConnectionInfo,
		string,
		string,
		bool,
	) (generaltypes.Databaser, error) {
		return stubDatabase{
			connectErr: connectErr,
			closeFn: func() {
				closeCalled = true
			},
		}, nil
	}

	err := verifyDatabaseConnection(context.Background(), deployment)
	if !errors.Is(err, connectErr) {
		t.Fatalf("expected connect error %v, got %v", connectErr, err)
	}
	if closeCalled {
		t.Fatal("expected readiness probe to avoid closing before connect succeeds")
	}
}

func TestVerifyDatabaseConnection_LocalBackendRequiresSuccessfulQuery(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteSecrets(deployment.Root(), &config.Secrets{
		DbPassword: localDefaultDatabasePassword,
	}); err != nil {
		t.Fatalf("failed to write local secrets: %v", err)
	}
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend: config.DeploymentBackendLocal,
		Connection: &config.DeploymentConnection{
			Host:                       "127.0.0.1",
			DisplayHost:                "localhost",
			DBPort:                     8563,
			UIPort:                     8443,
			Username:                   localDefaultDatabaseUser,
			InsecureSkipCertValidation: true,
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	originalNewExasolProbeConnection := newExasolProbeConnectionFn
	t.Cleanup(func() {
		newExasolProbeConnectionFn = originalNewExasolProbeConnection
	})

	execErr := errors.New("query failed")
	newExasolProbeConnectionFn = func(
		config.DeploymentDir,
		*config.ConnectionInfo,
		string,
		string,
		bool,
	) (generaltypes.Databaser, error) {
		return stubDatabase{execErr: execErr}, nil
	}

	err := verifyDatabaseConnection(context.Background(), deployment)
	if !errors.Is(err, execErr) {
		t.Fatalf("expected query error %v, got %v", execErr, err)
	}
}
