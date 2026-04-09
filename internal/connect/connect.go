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
)

type Opts struct {
	Username                   string
	Password                   string
	InsecureSkipCertValidation bool
}

//nolint:revive
func NewExasolConnection(
	deploymentDir string,
	username string,
	password string,
	insecureSkipCertValidation bool,
) (generaltypes.Databaser, error) {
	if password == "" {
		secrets, err := config.ReadSecrets(deploymentDir)
		if err != nil {
			return nil, fmt.Errorf("reading secrets: %w", err)
		}
		password = secrets.DbPassword
	}

	nodeDetails, err := config.ReadNodeDetails(deploymentDir)
	if err != nil {
		return nil, fmt.Errorf("reading node details: %w", err)
	}

	host, port, err := nodeDetails.GetDeploymentHostPort()
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment host-port: %w", err)
	}
	certFingerprint, err := nodeDetails.GetCertFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment tls certificate: %w", err)
	}

	optsFns := []exasol.OptFn{}
	if insecureSkipCertValidation {
		optsFns = append(optsFns, exasol.WithoutValidateServerCertificate)
	}

	database, err := exasol.New(username, password, host, certFingerprint, port, optsFns...)
	if err != nil {
		return nil, err
	}

	slog.Debug(
		"connecting to database",
		"username", username,
		"host", host,
		"port", port,
		"insecure_skip_cert_validation", insecureSkipCertValidation,
	)

	return database, nil
}

func Connect(ctx context.Context, opts *Opts, deploymentDir string) error {
	slog.Debug("running connect")

	database, err := NewExasolConnection(
		deploymentDir, opts.Username, opts.Password, opts.InsecureSkipCertValidation)
	if err != nil {
		return err
	}

	if err := database.Connect(ctx); err != nil {
		return err
	}

	defer database.Close()

	if isInteractiveStdin() {
		if err := printExitHint(os.Stderr); err != nil {
			return err
		}
	}

	return RunShell(func(input string) error {
		// The input string is expected to be trimmed of whitespace
		if input == "" {
			return nil
		}

		queryResult, err := database.Exec(ctx, input)
		if err != nil {
			return err
		}

		return printResult(queryResult)
	})
}

func isInteractiveStdin() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
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
