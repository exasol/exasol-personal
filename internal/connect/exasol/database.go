// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"

	"github.com/exasol/exasol-driver-go"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/util"
)

var ErrNoVersion = errors.New("the database didn't return version when queried")

const closePanicMsg = "tried to call Close before Connect on an instance of Exasol database"

type Database struct {
	connectionString string
	connect          types.ConnectFunc

	conn types.ExasolConnector
}

type opts struct {
	validateServerCertificate bool
	connect                   types.ConnectFunc
}

type OptFn func(*opts)

// WithoutValidateServerCertificate disables server
// certificate validation.
func WithoutValidateServerCertificate(opts *opts) {
	opts.validateServerCertificate = false
}

func WithConnectFunc(connect types.ConnectFunc) func(*opts) {
	return func(opts *opts) {
		opts.connect = connect
	}
}

func New(
	username, password, host, certFingerprint string,
	port int,
	optFns ...OptFn,
) (generaltypes.Databaser, error) {
	opts := &opts{
		validateServerCertificate: true,
		connect:                   defaultConnectFunc,
	}

	for _, optFn := range optFns {
		optFn(opts)
	}

	dsnConfigBuilder := exasol.NewConfig(username, password).
		Host(host).
		Port(port).
		CertificateFingerprint(certFingerprint).
		ValidateServerCertificate(opts.validateServerCertificate)

	return &Database{
		connectionString: dsnConfigBuilder.String(),
		connect:          opts.connect,
	}, nil
}

const WHITESPACE = `\s+`

var localImportRegex = regexp.MustCompile(
	`(?ims)^\s*IMPORT[\s(]+.+FROM` + WHITESPACE + `LOCAL` + WHITESPACE + `CSV.*$`,
)

// IsImportQuery returns true if the passed query is a file import query.
//
// Copied from https://github.com/exasol/exasol-driver-go/blob/main/internal/utils/helper.go
func isImportQuery(query string) bool {
	return localImportRegex.MatchString(query)
}

func defaultConnectFunc(input string) (types.ExasolConnector, error) {
	conn, err := (&exasol.ExasolDriver{}).Open(input)
	if err != nil {
		return nil, err
	}

	// Should always succeed unless the Go driver changes interface.
	return conn.(types.ExasolConnector), nil // nolint: revive,forcetypeassert
}

func (db *Database) Connect(ctx context.Context) error {
	slog.Debug("connecting to the database", "connection_string", db.connectionString)

	conn, err := db.connect(db.connectionString)
	if err != nil {
		return err
	}

	db.conn = conn

	version, err := db.version(ctx)
	if err != nil {
		// We can tolerate this failing. After all, printing the version
		// is more of a cosmetic functionality.
		slog.Warn("Couldn't get the database version", "err", err.Error())
		return nil
	}

	if util.IsInteractiveStdin() {
		return printVersion(os.Stderr, version)
	}

	return nil
}

func (db *Database) Close() error {
	slog.Debug("closing database connection")

	// Calling Close before Connect is an implementation error
	// and therefore should cause a panic.
	if db.conn == nil {
		panic(closePanicMsg)
	}

	return db.conn.Close()
}

func (db *Database) Exec(
	ctx context.Context,
	query string,
	maxRows int,
) (generaltypes.QueryResulter, error) {
	slog.Debug("executing query", "query", query, "max_rows", maxRows)

	// File import queries are executed through the driver's Exec because the
	// driver streams the local file; they never produce a result set.
	if isImportQuery(query) {
		_, err := db.conn.Exec(query, nil)
		return &QueryResult{columnNames: []string{}, rows: [][]string{}}, err
	}

	// QueryContext returns driver.Rows, which transparently fetches result-set
	// chunks (large results) and closes the server-side handle on Close.
	rows, err := db.conn.QueryContext(ctx, query, nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectRows(rows, maxRows)
}

// collectRows materializes up to maxRows rows from rows (maxRows == 0 means
// unlimited). When maxRows is reached and at least one further row exists, the
// result is flagged truncated and no additional rows are fetched.
func collectRows(rows driver.Rows, maxRows int) (*QueryResult, error) {
	columns := rows.Columns()
	result := &QueryResult{columnNames: columns, rows: [][]string{}}

	dest := make([]driver.Value, len(columns))

	for {
		if err := rows.Next(dest); err != nil {
			if errors.Is(err, io.EOF) {
				return result, nil
			}

			return nil, err
		}

		// One row beyond the cap proves there is more to show; stop fetching.
		if maxRows > 0 && len(result.rows) >= maxRows {
			result.truncated = true
			return result, nil
		}

		row := make([]string, len(columns))
		for i, value := range dest {
			row[i] = fmt.Sprint(value)
		}

		result.rows = append(result.rows, row)
	}
}

// version returns the database version.
func (db *Database) version(ctx context.Context) (string, error) {
	slog.Debug("getting the database version")

	queryResult, err := db.Exec(ctx,
		"SELECT param_value FROM exa_metadata WHERE param_name = 'databaseProductVersion'",
		0,
	)
	if err != nil {
		return "", err
	}

	rows := queryResult.Rows()

	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", ErrNoVersion
	}

	return rows[0][0], nil
}

func printVersion(output io.Writer, version string) error {
	_, err := fmt.Fprintln(output, "Exasol", version)

	return err
}
