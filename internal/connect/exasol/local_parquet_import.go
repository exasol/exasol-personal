// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/exasol/exasol-driver-go"
	"github.com/exasol/exasol-driver-go/pkg/connection"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

type parquetImportFunc func(
	ctx context.Context,
	database *sql.DB,
	schema, table, filePath string,
	options connection.ParquetImportOptions,
) (int64, error)

const stagingSuffixBytes = 8

// supportedParquetPath matches the file paths the driver's local import accepts.
var supportedParquetPath = regexp.MustCompile(`^[a-zA-Z0-9:<> /._\-~]+$`)

var errNoSessionConnection = errors.New(
	"the database connection does not support creating tables from local Parquet files",
)

type sessionConn interface {
	driver.Conn
	driver.ExecerContext
	driver.QueryerContext
}

// The pool must not close the user's session.
type borrowedConn struct {
	sessionConn
}

func (borrowedConn) Close() error { return nil }

type borrowedConnector struct {
	conn sessionConn
}

func (c borrowedConnector) Connect(context.Context) (driver.Conn, error) {
	return borrowedConn{c.conn}, nil
}

func (borrowedConnector) Driver() driver.Driver { return exasol.ExasolDriver{} }

func (db *Database) execLocalParquetTable(
	ctx context.Context,
	table *generaltypes.LocalParquetTable,
) (generaltypes.QueryResulter, error) {
	session, ok := db.conn.(sessionConn)
	if !ok {
		return nil, errNoSessionConnection
	}

	if err := checkParquetPath(table.Path); err != nil {
		return nil, err
	}

	pool := sql.OpenDB(borrowedConnector{conn: session})
	defer pool.Close()
	pool.SetMaxOpenConns(1)

	rows, err := db.createTableFromParquet(ctx, pool, table)

	return statementOnlyQueryResult(generaltypes.StatementTypeCreate, rows), err
}

// Staged because the driver keeps its table on failure; the user's own CREATE finishes it.
func (db *Database) createTableFromParquet(
	ctx context.Context,
	pool *sql.DB,
	table *generaltypes.LocalParquetTable,
) (int64, error) {
	target := qualifiedName(table.Schema, table.Table)

	if !table.Replace {
		exists, err := tableExists(ctx, pool, table.Schema, table.Table)
		if err != nil {
			return 0, err
		}
		if exists {
			return 0, fmt.Errorf(
				"table %s already exists; use CREATE OR REPLACE TABLE to replace it", target,
			)
		}
	}

	stagingTable, err := stagingTableName()
	if err != nil {
		return 0, err
	}
	staging := qualifiedName(table.Schema, stagingTable)

	rows, err := db.importParquet(
		ctx,
		pool,
		table.Schema,
		stagingTable,
		table.Path,
		connection.ParquetImportOptions{ColumnNames: connection.RawParquetNames},
	)
	if err != nil {
		return 0, errors.Join(err, dropStagingTable(ctx, pool, staging))
	}

	create := "CREATE TABLE "
	if table.Replace {
		create = "CREATE OR REPLACE TABLE "
	}
	//nolint:gosec,unqueryvet // quoted identifiers; the columns come from the file
	copyStaging := create + target + " AS SELECT * FROM " + staging
	if _, err := pool.ExecContext(ctx, copyStaging); err != nil {
		return 0, errors.Join(err, dropStagingTable(ctx, pool, staging))
	}

	if err := dropStagingTable(ctx, pool, staging); err != nil {
		return 0, fmt.Errorf("created table %s, but %w", target, err)
	}

	return rows, nil
}

func dropStagingTable(ctx context.Context, pool *sql.DB, staging string) error {
	// Cleanup must also run when the import was cancelled.
	_, err := pool.ExecContext(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS "+staging)
	if err != nil {
		return fmt.Errorf("could not remove the staging table %s: %w", staging, err)
	}

	return nil
}

func stagingTableName() (string, error) {
	suffix := make([]byte, stagingSuffixBytes)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}

	return "PARQUET_IMPORT_" + strings.ToUpper(hex.EncodeToString(suffix)), nil
}

func qualifiedName(schema, table string) string {
	return connection.QuoteIdentifier(schema) + "." + connection.QuoteIdentifier(table)
}

func tableExists(ctx context.Context, pool *sql.DB, schema, table string) (bool, error) {
	var count int
	err := pool.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM SYS.EXA_ALL_TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
		schema,
		table,
	).Scan(&count)

	return count > 0, err
}

// Rejects up front what the driver would reject with a generic error.
func checkParquetPath(path string) error {
	if strings.Contains(path, `\`) {
		return fmt.Errorf("use / as the separator in the local Parquet file path: %s", path)
	}
	if !supportedParquetPath.MatchString(path) {
		return fmt.Errorf(
			"the local Parquet file path may only contain ASCII letters, digits, spaces "+
				"and the characters : < > / . _ - ~: %s",
			path,
		)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open local Parquet file: %w", err)
	}

	return file.Close()
}
