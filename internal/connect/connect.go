// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect/exasol"
	"github.com/exasol/exasol-personal/internal/connect/tablewriter"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/util"
)

type Opts struct {
	Username                   string
	Password                   string
	InsecureSkipCertValidation bool
	ExecuteOnSemicolon         bool
}

//nolint:revive
func NewExasolConnection(
	deployment config.DeploymentDir,
	username string,
	password string,
	insecureSkipCertValidation bool,
) (generaltypes.Databaser, error) {
	if password == "" {
		secrets, err := config.ReadSecrets(deployment)
		if err != nil {
			return nil, fmt.Errorf("reading secrets: %w", err)
		}
		password = secrets.DbPassword
	}

	connectionInfo, err := config.ResolveConnectionInfo(deployment)
	if err != nil {
		return nil, fmt.Errorf("reading deployment connection info: %w", err)
	}

	optsFns := []exasol.OptFn{}
	if insecureSkipCertValidation || connectionInfo.InsecureSkipCertValidation {
		optsFns = append(optsFns, exasol.WithoutValidateServerCertificate)
	}

	database, err := exasol.New(
		username,
		password,
		connectionInfo.Host,
		connectionInfo.CertFingerprint,
		connectionInfo.DBPort,
		optsFns...,
	)
	if err != nil {
		return nil, err
	}

	slog.Debug(
		"connecting to database",
		"username", username,
		"host", connectionInfo.Host,
		"port", connectionInfo.DBPort,
		"insecure_skip_cert_validation", insecureSkipCertValidation,
	)

	return database, nil
}

func Connect(ctx context.Context, opts *Opts, deployment config.DeploymentDir) error {
	slog.Debug("running connect")

	database, err := NewExasolConnection(
		deployment, opts.Username, opts.Password, opts.InsecureSkipCertValidation)
	if err != nil {
		return err
	}

	if err := database.Connect(ctx); err != nil {
		return err
	}

	defer database.Close()

	if util.IsInteractiveStdin() {
		if err := printExitHint(os.Stderr); err != nil {
			return err
		}
	}

	return RunShellWithOpts(func(input string) error {
		// The input string is expected to be trimmed of whitespace
		if input == "" {
			return nil
		}

		queryResult, err := database.Exec(ctx, input)
		if err != nil {
			return err
		}

		return printResult(queryResult)
	}, ShellOpts{ExecuteOnSemicolon: opts.ExecuteOnSemicolon})
}

func printExitHint(output io.Writer) error {
	_, err := fmt.Fprintln(output, "Type \"exit\" to exit the shell")

	return err
}

func printResult(queryResult generaltypes.QueryResulter) error {
	rows := queryResult.Rows()

	slog.Debug("printing query result", "num_rows", len(rows))

	table := tablewriter.New(os.Stdout)
	table.SetHeader(queryResult.ColumnNames())

	if err := table.SetRows(rows); err != nil {
		return err
	}

	return table.Render()
}
