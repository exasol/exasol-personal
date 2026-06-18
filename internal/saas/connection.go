// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"fmt"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect/exasol"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

// DBFactory opens (but does not connect) a database handle authenticated with the
// SaaS access token. It is injectable so the connection test and migration engine
// can be exercised with a fake.
type DBFactory func(token, host string, port int) (generaltypes.Databaser, error)

// DefaultDBFactory builds a real Exasol database handle. SaaS authenticates via
// the personal access token used as an OpenID refresh token (the driver redeems
// it during login; no username/password). SaaS clusters present an internal
// (non-public) certificate, so server-certificate verification is skipped while
// TLS encryption stays on.
func DefaultDBFactory(token, host string, port int) (generaltypes.Databaser, error) {
	return exasol.NewWithRefreshToken(token, host, port, exasol.WithoutValidateServerCertificate)
}

// connectTarget opens and connects to the SaaS target database using the token.
func connectTarget(
	ctx context.Context,
	newDB DBFactory,
	target *config.DeploymentSaaS,
	token string,
) (generaltypes.Databaser, error) {
	database, err := newDB(token, target.Host, target.Port)
	if err != nil {
		return nil, fmt.Errorf("preparing target connection: %w", err)
	}
	if err := database.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting to saas target: %w", err)
	}

	return database, nil
}

// targetConnectionString builds the ODBC-style connection string the source
// database uses for EXPORT ... INTO EXA. TLS encryption stays on, but server
// certificate verification is disabled because SaaS clusters present an internal
// (non-public) certificate.
func targetConnectionString(target *config.DeploymentSaaS) string {
	return fmt.Sprintf(
		"%s:%d;Encryption=Y;SSLCertificate=SSL_VERIFY_NONE",
		target.Host, target.Port,
	)
}
