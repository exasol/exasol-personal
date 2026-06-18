// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
)

// ErrNoToken indicates no SaaS token is defined for the deployment.
var ErrNoToken = errors.New("no saas token defined")

// TokenEnv supplies the SaaS token from the environment. When set, it takes
// precedence over a token stored in the deployment secrets.
const TokenEnv = "EXASOL_SAAS_TOKEN" //nolint:gosec // env var name, not a credential

// noTokenHint is shown when a token-gated command runs without a token.
//
//nolint:gosec // user-facing hint, not a credential
const noTokenHint = "no SaaS token defined — set the " + TokenEnv + " environment variable"

// LoadToken returns the SaaS token from the environment (preferred) or the
// deployment secrets, or ErrNoToken if none is defined.
func LoadToken(deployment config.DeploymentDir) (string, error) {
	if env := strings.TrimSpace(os.Getenv(TokenEnv)); env != "" {
		return env, nil
	}

	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		return "", err
	}

	token := strings.TrimSpace(secrets.SaaSToken)
	if token == "" {
		return "", ErrNoToken
	}

	return token, nil
}

// RequireToken returns the stored token or a user-facing error when missing.
func RequireToken(deployment config.DeploymentDir) (string, error) {
	token, err := LoadToken(deployment)
	if errors.Is(err, ErrNoToken) {
		return "", errors.New(noTokenHint)
	}
	if err != nil {
		return "", err
	}

	return token, nil
}

// SaveToken persists the SaaS token into the deployment secrets, preserving the
// other secrets already present.
func SaveToken(deployment config.DeploymentDir, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("refusing to store an empty saas token")
	}

	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		return err
	}

	secrets.SaaSToken = token

	return config.WriteSecrets(deployment.Root(), secrets)
}

// ClearToken removes the stored SaaS token.
func ClearToken(deployment config.DeploymentDir) error {
	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		return err
	}

	secrets.SaaSToken = ""

	return config.WriteSecrets(deployment.Root(), secrets)
}

// MaskToken renders a token for display, revealing only the last few characters.
func MaskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "(none)"
	}

	const revealed = 4
	if len(token) <= revealed {
		return strings.Repeat("*", len(token))
	}

	return fmt.Sprintf(
		"%s%s",
		strings.Repeat("*", len(token)-revealed),
		token[len(token)-revealed:],
	)
}
