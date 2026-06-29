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
	"time"

	"github.com/exasol/exasol-driver-go"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/exasol/exasol-personal/internal/util"
)

var (
	ErrNoVersion   = errors.New("the database didn't return version when queried")
	ErrNoSessionID = errors.New("the database didn't return session id when queried")
)

const closePanicMsg = "tried to call Close before Connect on an instance of Exasol database"

type Database struct {
	connectionString string
	connect          types.ConnectFunc

	conn              types.ExasolConnector
	sessionID         *string
	sessionIDResolved bool
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
	statementType := generaltypes.ClassifyStatement(query)

	// Non-result statements use the driver's Exec path. IMPORT statements also
	// belong here; LOCAL IMPORT streams a client-side file and never returns a
	// result set.
	if statementType.UsesExecPath() {
		result, err := db.conn.Exec(query, nil)
		return statementOnlyQueryResult(
			statementType,
			rowsAffected(result),
		), db.wrapExecutionError(ctx, err)
	}

	// QueryContext returns driver.Rows, which transparently fetches result-set
	// chunks (large results) and closes the server-side handle on Close.
	rows, err := db.conn.QueryContext(ctx, query, nil)
	if err != nil {
		return nil, db.wrapExecutionError(ctx, err)
	}
	defer rows.Close()

	result, err := collectRows(rows, maxRows, statementType)
	if err != nil {
		return nil, db.wrapExecutionError(ctx, err)
	}

	return result, nil
}

// collectRows materializes up to maxRows rows from rows (maxRows == 0 means
// unlimited). When maxRows is reached and at least one further row exists, the
// result is flagged truncated and no additional rows are fetched.
func collectRows(
	rows driver.Rows,
	maxRows int,
	statementType generaltypes.StatementType,
) (*QueryResult, error) {
	columns := rows.Columns()
	result := &QueryResult{
		columnNames:   columns,
		rows:          [][]string{},
		values:        [][]any{},
		statementType: statementType,
	}

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
		values := make([]any, len(columns))
		for i, value := range dest {
			row[i] = fmt.Sprint(value)
			values[i] = jsonValue(value)
		}

		result.rows = append(result.rows, row)
		result.values = append(result.values, values)
	}
}

func statementOnlyQueryResult(
	statementType generaltypes.StatementType,
	rowsAffected int64,
) *QueryResult {
	return &QueryResult{
		columnNames:   []string{},
		rows:          [][]string{},
		values:        [][]any{},
		statementType: statementType,
		rowsAffected:  rowsAffected,
	}
}

func rowsAffected(result driver.Result) int64 {
	if result == nil {
		return 0
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0
	}

	return rowsAffected
}

func (db *Database) wrapExecutionError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	structured := generaltypes.StructuredSQLErrorFromError(err)
	if sessionID := db.ensureSessionID(ctx); sessionID != nil {
		// Prefer the live session identifier from the connection when available.
		// Message-derived values are only a fallback for errors that already carry one.
		structured.SessionID = sessionID
	}

	return executionError{cause: err, structured: structured}
}

func (db *Database) ensureSessionID(ctx context.Context) *string {
	if db.sessionID != nil || db.sessionIDResolved {
		return db.sessionID
	}

	// Only attempt the lazy lookup once per connection. On failure we keep the
	// execution error intact instead of issuing extra metadata queries.
	db.sessionIDResolved = true

	sessionID, err := db.currentSessionID(ctx)
	if err != nil {
		slog.Debug("Couldn't get the database session id", "err", err.Error())
		return nil
	}

	db.sessionID = sessionID

	return db.sessionID
}

func jsonValue(value driver.Value) any {
	switch typedValue := value.(type) {
	case nil, string, bool, int64, float64:
		return typedValue
	case []byte:
		return string(typedValue)
	case time.Time:
		return typedValue.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(typedValue)
	}
}

// version returns the database version.
func (db *Database) version(ctx context.Context) (string, error) {
	slog.Debug("getting the database version")

	version, found, err := db.queryScalar(ctx,
		"SELECT param_value FROM exa_metadata WHERE param_name = 'databaseProductVersion'",
	)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNoVersion
	}

	return version, nil
}

func (db *Database) currentSessionID(ctx context.Context) (*string, error) {
	slog.Debug("getting the database session id")

	sessionID, found, err := db.queryScalar(ctx, "SELECT CURRENT_SESSION")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNoSessionID
	}

	return &sessionID, nil
}

// queryScalar runs query and returns the first cell of the first row as a
// string. The boolean reports whether a row and non-nil value were present.
func (db *Database) queryScalar(ctx context.Context, query string) (string, bool, error) {
	rows, err := db.conn.QueryContext(ctx, query, nil)
	if err != nil {
		return "", false, err
	}
	if rows == nil {
		return "", false, nil
	}
	defer rows.Close()

	dest := make([]driver.Value, len(rows.Columns()))
	if err := rows.Next(dest); err != nil {
		if errors.Is(err, io.EOF) {
			return "", false, nil
		}

		return "", false, err
	}

	if len(dest) == 0 || dest[0] == nil {
		return "", false, nil
	}

	return fmt.Sprint(dest[0]), true, nil
}

func printVersion(output io.Writer, version string) error {
	_, err := fmt.Fprintln(output, "Exasol", version)

	return err
}
