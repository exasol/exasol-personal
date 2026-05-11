// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/connect/exasol"
	"github.com/exasol/exasol-personal/internal/connect/tablewriter"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/util"
)

type JSONFormat string

const (
	JSONFormatPretty  JSONFormat = "pretty"
	JSONFormatCompact JSONFormat = "compact"
)

func (format JSONFormat) String() string {
	return string(format)
}

func ParseJSONFormat(format string) (JSONFormat, error) {
	normalized := JSONFormat(strings.ToLower(strings.TrimSpace(format)))

	switch normalized {
	case "", JSONFormatPretty:
		return JSONFormatPretty, nil
	case JSONFormatCompact:
		return JSONFormatCompact, nil
	default:
		return "", fmt.Errorf(
			"invalid json format %q (expected one of: %s, %s)",
			format,
			JSONFormatPretty,
			JSONFormatCompact,
		)
	}
}

type Opts struct {
	Username                   string
	Password                   string
	InsecureSkipCertValidation bool
	ExecuteOnSemicolon         bool
	OutputJSON                 bool
	JSONFormat                 JSONFormat
}

type jsonQueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type resultPrinter func(io.Writer, generaltypes.QueryResulter) error

//nolint:revive
func NewExasolConnection(
	deployment config.DeploymentDir,
	connectionInfo *config.ConnectionInfo,
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
	if connectionInfo == nil {
		return nil, errors.New("reading deployment connection info: missing connection info")
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

func Connect(
	ctx context.Context,
	opts *Opts,
	deployment config.DeploymentDir,
	connectionInfo *config.ConnectionInfo,
) error {
	slog.Debug("running connect")

	database, err := NewExasolConnection(
		deployment,
		connectionInfo,
		opts.Username,
		opts.Password,
		opts.InsecureSkipCertValidation,
	)
	if err != nil {
		return err
	}

	if err := database.Connect(ctx); err != nil {
		return err
	}

	defer database.Close()

	output := os.Stdout
	printer := printResultTable
	if opts.OutputJSON {
		printer = newJSONResultPrinter(opts.JSONFormat)
	}

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

		return printer(output, queryResult)
	}, ShellOpts{ExecuteOnSemicolon: opts.ExecuteOnSemicolon})
}

func printExitHint(output io.Writer) error {
	_, err := fmt.Fprintln(output, "Type \"exit\" to exit the shell")

	return err
}

func normalizeJSONFormat(format JSONFormat) JSONFormat {
	switch JSONFormat(strings.ToLower(strings.TrimSpace(format.String()))) {
	case JSONFormatPretty:
		return JSONFormatPretty
	case JSONFormatCompact:
		return JSONFormatCompact
	default:
		return JSONFormatPretty
	}
}

func newJSONResultPrinter(jsonFormat JSONFormat) resultPrinter {
	format := normalizeJSONFormat(jsonFormat)

	return func(output io.Writer, queryResult generaltypes.QueryResulter) error {
		return printResultJSON(output, queryResult, format)
	}
}

func printResultJSON(
	output io.Writer,
	queryResult generaltypes.QueryResulter,
	jsonFormat JSONFormat,
) error {
	encoder := json.NewEncoder(output)
	if normalizeJSONFormat(jsonFormat) == JSONFormatPretty {
		encoder.SetIndent("", "  ")
	}

	return encoder.Encode(jsonQueryResult{
		Columns: queryResult.ColumnNames(),
		Rows:    queryResult.Rows(),
	})
}

func printResultTable(output io.Writer, queryResult generaltypes.QueryResulter) error {
	rows := queryResult.Rows()

	slog.Debug("printing query result", "num_rows", len(rows))

	table := tablewriter.New(output)
	table.SetHeader(queryResult.ColumnNames())

	if err := table.SetRows(rows); err != nil {
		return err
	}

	return table.Render()
}
