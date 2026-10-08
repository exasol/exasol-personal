// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect"
	"github.com/exasol/exasol-personal/internal/localruntime"
)

const (
	localCredentialMarkerDirMode       = 0o750
	localCredentialMarkerFileMode      = 0o600
	localCredentialVerificationTimeout = 30 * time.Second
)

var (
	errLocalGeneratedSecretsUnavailable = errors.New(
		"the generated database password for this local deployment is missing from secrets.json",
	)
	errLocalStoredCredentialRejected = errors.New(
		"the local database rejected the password stored in secrets.json",
	)
	verifyLocalStoredCredentialFn = verifyLocalStoredCredential
)

// localCredentialMarker records, without the secret, that the database password
// was generated and whether a login with it has succeeded since. Verification
// stays pending across retries until a login confirms the stored password.
type localCredentialMarker struct {
	DbPasswordGenerated bool `json:"dbPasswordGenerated"`
	DbPasswordVerified  bool `json:"dbPasswordVerified"`
}

func readLocalCredentialMarker(deployment config.DeploymentDir) (localCredentialMarker, error) {
	var marker localCredentialMarker
	data, err := os.ReadFile(localruntime.CredentialMarkerPath(deployment))
	if errors.Is(err, os.ErrNotExist) {
		return marker, nil
	}
	if err != nil {
		return marker, fmt.Errorf("failed to read local credential marker: %w", err)
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return marker, fmt.Errorf("failed to parse local credential marker: %w", err)
	}

	return marker, nil
}

func writeLocalCredentialMarker(
	deployment config.DeploymentDir,
	marker localCredentialMarker,
) error {
	path := localruntime.CredentialMarkerPath(deployment)
	if err := os.MkdirAll(filepath.Dir(path), localCredentialMarkerDirMode); err != nil {
		return fmt.Errorf("failed to create local credential marker directory: %w", err)
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomically(path, data, localCredentialMarkerFileMode); err != nil {
		return fmt.Errorf("failed to write local credential marker: %w", err)
	}

	return nil
}

func localDatabasePasswordGenerated(deployment config.DeploymentDir) (bool, error) {
	marker, err := readLocalCredentialMarker(deployment)

	return marker.DbPasswordGenerated, err
}

func localCredentialVerificationPending(deployment config.DeploymentDir) (bool, error) {
	marker, err := readLocalCredentialMarker(deployment)

	return marker.DbPasswordGenerated && !marker.DbPasswordVerified, err
}

func markLocalDatabasePasswordGenerated(deployment config.DeploymentDir) error {
	return writeLocalCredentialMarker(deployment, localCredentialMarker{DbPasswordGenerated: true})
}

func markLocalDatabasePasswordVerified(deployment config.DeploymentDir) error {
	return writeLocalCredentialMarker(deployment, localCredentialMarker{
		DbPasswordGenerated: true,
		DbPasswordVerified:  true,
	})
}

// resolveLocalInitialPassword returns the password a fresh local database is
// initialized with. A generated password already stored for this deployment is
// reused, so a retried first initialization keeps matching secrets.json; any
// other stored value is replaced, because a fresh database has no password yet.
func resolveLocalInitialPassword(deployment config.DeploymentDir) (string, error) {
	generated, err := localDatabasePasswordGenerated(deployment)
	if err != nil {
		return "", err
	}
	secrets, readErr := config.ReadSecrets(deployment)
	if readErr != nil {
		secrets = &config.Secrets{}
	}
	if generated && secrets.DbPassword != "" {
		return secrets.DbPassword, nil
	}

	password, err := generateDatabasePassword()
	if err != nil {
		return "", fmt.Errorf("failed to generate the local database password: %w", err)
	}
	secrets.DbPassword = password
	if err := config.WriteSecrets(deployment.Root(), secrets); err != nil {
		return "", err
	}
	if err := markLocalDatabasePasswordGenerated(deployment); err != nil {
		return "", err
	}

	return password, nil
}

// verifyLocalStoredCredential logs in once with the stored password, so a first
// initialization that did not apply it fails instead of being recorded as running.
func verifyLocalStoredCredential(ctx context.Context, deployment config.DeploymentDir) error {
	ctx, cancel := context.WithTimeout(ctx, localCredentialVerificationTimeout)
	defer cancel()

	err := connect.WithSilencedDriverErrors(func() error {
		connectionInfo, err := config.ResolveConnectionInfo(deployment)
		if err != nil {
			return err
		}
		database, err := newExasolConnectionFn(
			deployment, connectionInfo, connectionInfo.Username, "", true,
		)
		if err != nil {
			return err
		}
		if err := database.Connect(ctx); err != nil {
			return err
		}

		return database.Close()
	})

	return classifyLocalCredentialCheck(err)
}

func classifyLocalCredentialCheck(err error) error {
	if err == nil {
		return nil
	}
	if isAuthenticationFailure(err) {
		return fmt.Errorf(
			"%w; run `exasol destroy` and then `exasol deploy` to recreate the "+
				"deployment with a new password",
			errLocalStoredCredentialRejected,
		)
	}

	return fmt.Errorf("failed to verify the local database password: %w", err)
}
