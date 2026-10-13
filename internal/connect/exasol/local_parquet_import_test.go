// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-driver-go/pkg/connection"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types/typesfakes"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/stretchr/testify/require"
)

// fakeSessionConn is a driver connection that answers table-existence queries
// and records every executed statement.
type fakeSessionConn struct {
	tableExists bool
	failOn      string
	cancelAfter string
	cancel      context.CancelFunc
	statements  []string
	tableQuery  []driver.NamedValue
	closed      bool
}

func (*fakeSessionConn) Prepare(string) (driver.Stmt, error) { return nil, errTest }

func (*fakeSessionConn) Begin() (driver.Tx, error) { return nil, errTest }

func (f *fakeSessionConn) Close() error {
	f.closed = true

	return nil
}

func (f *fakeSessionConn) Exec(query string, _ []driver.Value) (driver.Result, error) {
	f.statements = append(f.statements, query)

	return fakeResult{}, nil
}

func (f *fakeSessionConn) ExecContext(
	_ context.Context, query string, _ []driver.NamedValue,
) (driver.Result, error) {
	f.statements = append(f.statements, query)
	if f.failOn != "" && strings.HasPrefix(query, f.failOn) {
		return nil, errTest
	}
	if f.cancelAfter != "" && strings.HasPrefix(query, f.cancelAfter) {
		f.cancel()
	}

	return fakeResult{}, nil
}

func (f *fakeSessionConn) QueryContext(
	_ context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	if !strings.Contains(query, "EXA_ALL_TABLES") {
		return nil, errTest
	}

	f.tableQuery = args
	count := int64(0)
	if f.tableExists {
		count = 1
	}

	return &fakeRows{columns: []string{"COUNT"}, data: [][]driver.Value{{count}}}, nil
}

type parquetImportCall struct {
	schema, table, filePath string
	options                 connection.ParquetImportOptions
}

type parquetImportHarness struct {
	session  *fakeSessionConn
	database *Database
	calls    []parquetImportCall
}

func newParquetImportHarness(t *testing.T, session types.ExasolConnector) *parquetImportHarness {
	t.Helper()

	harness := &parquetImportHarness{}
	if fake, ok := session.(*fakeSessionConn); ok {
		harness.session = fake
	}

	connect := func(string) (types.ExasolConnector, error) { return session, nil }
	database, ok := testDatabaseFactory(t, connect).(*Database)
	require.True(t, ok)
	require.NoError(t, database.Connect(t.Context()))

	database.importParquet = func(
		ctx context.Context,
		pool *sql.DB,
		schema, table, filePath string,
		options connection.ParquetImportOptions,
	) (int64, error) {
		harness.calls = append(harness.calls, parquetImportCall{schema, table, filePath, options})
		if _, err := pool.ExecContext(ctx, "IMPORT"); err != nil {
			return 0, err
		}

		return 3, nil
	}
	harness.database = database

	return harness
}

func writeParquetFile(t *testing.T) string {
	t.Helper()

	path := filepath.ToSlash(filepath.Join(t.TempDir(), "sales.parquet"))
	require.NoError(t, os.WriteFile(path, []byte("PAR1"), 0o600))

	return path
}

const createSalesFrom = "CREATE TABLE demo.sales AS (IMPORT FROM LOCAL PARQUET FILE '"

const replaceSalesFrom = "CREATE OR REPLACE TABLE demo.sales AS (IMPORT FROM LOCAL PARQUET FILE '"

func (h *parquetImportHarness) staging(t *testing.T) string {
	t.Helper()
	require.Len(t, h.calls, 1)
	require.True(t, strings.HasPrefix(h.calls[0].table, "PARQUET_IMPORT_"), h.calls[0].table)

	return `"DEMO"."` + h.calls[0].table + `"`
}

func copyFrom(create, staging string) string {
	//nolint:unqueryvet // mirrors the production statement
	return create + " AS SELECT * FROM " + staging
}

func TestExecCreatesTableFromLocalParquetFileOnTheSession(t *testing.T) {
	t.Parallel()

	// Given
	harness := newParquetImportHarness(t, &fakeSessionConn{})
	path := writeParquetFile(t)

	// When
	result, err := harness.database.Exec(t.Context(), createSalesFrom+path+"')", 0)

	// Then
	require.NoError(t, err)
	require.Equal(t, generaltypes.StatementTypeCreate, result.StatementType())
	require.Equal(t, int64(3), result.RowsAffected())
	staging := harness.staging(t)
	require.Equal(t, "DEMO", harness.calls[0].schema)
	require.Equal(t, path, harness.calls[0].filePath)
	require.Equal(
		t,
		connection.ParquetImportOptions{ColumnNames: connection.RawParquetNames},
		harness.calls[0].options,
	)
	require.Equal(t, "DEMO", harness.session.tableQuery[0].Value)
	require.Equal(t, "SALES", harness.session.tableQuery[1].Value)
	require.Equal(t, []string{
		"IMPORT",
		copyFrom(`CREATE TABLE "DEMO"."SALES"`, staging),
		"DROP TABLE IF EXISTS " + staging,
	}, harness.session.statements)
	require.False(t, harness.session.closed)
}

func TestExecRejectsExistingTableWithoutReplace(t *testing.T) {
	t.Parallel()

	// Given
	harness := newParquetImportHarness(t, &fakeSessionConn{tableExists: true})
	path := writeParquetFile(t)

	// When
	_, err := harness.database.Exec(t.Context(), createSalesFrom+path+"')", 0)

	// Then
	require.ErrorContains(t, err, `table "DEMO"."SALES" already exists`)
	require.Empty(t, harness.calls)
	require.Empty(t, harness.session.statements)
}

func TestExecReplacesTableWithOneCreateStatement(t *testing.T) {
	t.Parallel()

	// Given
	harness := newParquetImportHarness(t, &fakeSessionConn{tableExists: true})
	path := writeParquetFile(t)

	// When
	_, err := harness.database.Exec(t.Context(), replaceSalesFrom+path+"')", 0)

	// Then
	require.NoError(t, err)
	staging := harness.staging(t)
	require.Equal(t, []string{
		"IMPORT",
		copyFrom(`CREATE OR REPLACE TABLE "DEMO"."SALES"`, staging),
		"DROP TABLE IF EXISTS " + staging,
	}, harness.session.statements)
	require.Nil(t, harness.session.tableQuery)
}

func TestExecRemovesStagingTableWhenCreatingTheTableFails(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		statement string
		failOn    string
		create    string
	}{
		{name: "import fails", statement: createSalesFrom, failOn: "IMPORT"},
		{
			name:      "create fails",
			statement: createSalesFrom,
			failOn:    "CREATE TABLE",
			create:    `CREATE TABLE "DEMO"."SALES"`,
		},
		{
			name:      "replace fails",
			statement: replaceSalesFrom,
			failOn:    "CREATE OR REPLACE",
			create:    `CREATE OR REPLACE TABLE "DEMO"."SALES"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			harness := newParquetImportHarness(t, &fakeSessionConn{failOn: test.failOn})
			path := writeParquetFile(t)

			// When
			_, err := harness.database.Exec(t.Context(), test.statement+path+"')", 0)

			// Then
			require.ErrorIs(t, err, errTest)
			staging := harness.staging(t)
			expected := []string{"IMPORT"}
			if test.create != "" {
				expected = append(expected, copyFrom(test.create, staging))
			}
			expected = append(expected, "DROP TABLE IF EXISTS "+staging)
			require.Equal(t, expected, harness.session.statements)
		})
	}
}

func TestExecReportsStagingTableLeftAfterCreatingTheTable(t *testing.T) {
	t.Parallel()

	// Given
	harness := newParquetImportHarness(t, &fakeSessionConn{failOn: "DROP TABLE IF EXISTS"})
	path := writeParquetFile(t)

	// When
	_, err := harness.database.Exec(t.Context(), createSalesFrom+path+"')", 0)

	// Then
	require.ErrorIs(t, err, errTest)
	require.ErrorContains(
		t,
		err,
		`created table "DEMO"."SALES", but could not remove the staging table `+harness.staging(t),
	)
}

func TestExecRemovesStagingTableWhenCancelledAfterImport(t *testing.T) {
	t.Parallel()

	// Given
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	session := &fakeSessionConn{cancelAfter: "IMPORT", cancel: cancel}
	harness := newParquetImportHarness(t, session)
	path := writeParquetFile(t)

	// When
	_, err := harness.database.Exec(ctx, createSalesFrom+path+"')", 0)

	// Then
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{
		"IMPORT",
		"DROP TABLE IF EXISTS " + harness.staging(t),
	}, session.statements)
}

func TestExecLeavesTableUntouchedForUnusablePath(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		path          string
		expectedError string
	}{
		{
			name:          "missing file",
			path:          "/does/not/exist.parquet",
			expectedError: "cannot open local Parquet file",
		},
		{
			name:          "backslash separator",
			path:          `C:\data\sales.parquet`,
			expectedError: "use / as the separator",
		},
		{
			name:          "parentheses",
			path:          "/data/sales(2024).parquet",
			expectedError: "may only contain ASCII letters",
		},
		{
			name:          "non-ASCII letter",
			path:          "/home/j\u00fcrgen/sales.parquet",
			expectedError: "may only contain ASCII letters",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Given
			harness := newParquetImportHarness(t, &fakeSessionConn{tableExists: true})

			// When
			_, err := harness.database.Exec(t.Context(), replaceSalesFrom+test.path+"')", 0)

			// Then
			require.ErrorContains(t, err, test.expectedError)
			require.Empty(t, harness.calls)
			require.Empty(t, harness.session.statements)
		})
	}
}

func TestExecRejectsUnsupportedLocalParquetTableShape(t *testing.T) {
	t.Parallel()

	// Given
	harness := newParquetImportHarness(t, &fakeSessionConn{})

	// When
	_, err := harness.database.Exec(
		t.Context(),
		"CREATE TABLE s.t AS SELECT id FROM (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		0,
	)

	// Then
	require.ErrorIs(t, err, generaltypes.ErrUnsupportedLocalParquetTable)
	require.Empty(t, harness.calls)
	require.Empty(t, harness.session.statements)
}

func TestExecRequiresSessionConnectionForLocalParquetTable(t *testing.T) {
	t.Parallel()

	// Given
	fakeConnector := &typesfakes.FakeExasolConnector{}
	fakeConnector.QueryContextReturns(nil, errTest)
	harness := newParquetImportHarness(t, fakeConnector)
	path := writeParquetFile(t)

	// When
	_, err := harness.database.Exec(
		t.Context(),
		"CREATE TABLE demo.sales AS (IMPORT FROM LOCAL PARQUET FILE '"+path+"')",
		0,
	)

	// Then
	require.ErrorIs(t, err, errNoSessionConnection)
	require.Empty(t, harness.calls)
	require.Zero(t, fakeConnector.ExecCallCount())
}
