// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"context"
	"encoding/csv"
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
	OutputCSV                  bool
	JSONFormat                 JSONFormat
	// Command holds inline SQL passed via --command. When set, the statements
	// are executed non-interactively and the shell is not started.
	Command string
	// File holds the path to a SQL script passed via --file. When set, the
	// file's statements are executed non-interactively and the shell is not
	// started.
	File string
	// MaxRows caps the number of rows displayed per query. A negative value
	// means "unset" (use the per-mode default); 0 means unlimited.
	MaxRows int
}

// MaxRowsUnset is the sentinel for a MaxRows that was not set on the command
// line, leaving the limit to be derived from the session mode.
const MaxRowsUnset = -1

// interactivePreviewMaxRows is the default row cap for interactive sessions.
const interactivePreviewMaxRows = 100

// effectiveMaxRows returns the explicit row limit when one was set, otherwise
// the mode default. A negative requested value means "unset".
func effectiveMaxRows(requested, modeDefault int) int {
	if requested >= 0 {
		return requested
	}

	return modeDefault
}

type jsonQueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
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

	// Resolve any non-interactive SQL source up front so that an unreadable
	// --file fails fast without opening a database connection.
	nonInteractiveSQL, nonInteractive, err := resolveNonInteractiveSQL(opts)
	if err != nil {
		return err
	}

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
	if opts.OutputCSV {
		printer = printResultCSV
	} else if opts.OutputJSON {
		printer = newJSONResultPrinter(opts.JSONFormat)
	}

	// The interactive preview cap applies only when an interactive shell is
	// actually started. --command/--file and piped input return everything by
	// default, so they behave as non-interactive (unlimited) unless --max-rows
	// is set explicitly.
	interactiveShell := !nonInteractive && util.IsInteractiveStdin()

	modeDefault := 0
	if interactiveShell {
		modeDefault = interactivePreviewMaxRows
	}
	maxRows := effectiveMaxRows(opts.MaxRows, modeDefault)

	processInput := func(input string) error {
		// The input string is expected to be trimmed of whitespace
		if input == "" {
			return nil
		}

		queryResult, err := database.Exec(ctx, input, maxRows)
		if err != nil {
			return err
		}

		if err := printer(output, queryResult); err != nil {
			return err
		}

		if queryResult.Truncated() {
			return printTruncationFooter(os.Stderr, len(queryResult.Rows()))
		}

		return nil
	}

	if nonInteractive {
		return runStatements(nonInteractiveSQL, processInput)
	}

	if interactiveShell {
		if err := printExitHint(os.Stderr); err != nil {
			return err
		}
	}

	return RunShellWithOpts(processInput, ShellOpts{ExecuteOnSemicolon: opts.ExecuteOnSemicolon})
}

// resolveNonInteractiveSQL returns the SQL to run non-interactively, derived
// from --file or --command. The boolean reports whether a non-interactive
// source was supplied; when false, the caller falls back to the interactive
// shell. A --file that cannot be read yields an error before any database
// connection is attempted.
func resolveNonInteractiveSQL(opts *Opts) (string, bool, error) {
	switch {
	case opts.File != "":
		contents, err := os.ReadFile(opts.File)
		if err != nil {
			return "", false, fmt.Errorf("reading SQL file %q: %w", opts.File, err)
		}

		return string(contents), true, nil
	case opts.Command != "":
		return opts.Command, true, nil
	default:
		return "", false, nil
	}
}

// printTruncationFooter notifies the user, via stderr, that the displayed
// result was capped. It is written to stderr so it never corrupts stdout
// output such as JSON.
func printTruncationFooter(output io.Writer, shown int) error {
	_, err := fmt.Fprintf(
		output,
		"-- showing first %d rows (output truncated; use --max-rows 0 to see all)\n",
		shown,
	)

	return err
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
	encoder.SetEscapeHTML(false)
	if normalizeJSONFormat(jsonFormat) == JSONFormatPretty {
		encoder.SetIndent("", "  ")
	}

	return encoder.Encode(jsonQueryResult{
		Columns: queryResult.ColumnNames(),
		Rows:    queryResult.Values(),
	})
}

func printResultCSV(output io.Writer, queryResult generaltypes.QueryResulter) error {
	columns := queryResult.ColumnNames()
	if len(columns) == 0 {
		return nil
	}

	writer := csv.NewWriter(output)
	if err := writer.Write(columns); err != nil {
		return err
	}

	for _, row := range queryResult.Values() {
		record := make([]string, len(row))
		for i, value := range row {
			if value != nil {
				record[i] = fmt.Sprint(value)
			}
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	writer.Flush()

	return writer.Error()
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
