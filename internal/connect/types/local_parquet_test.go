// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLocalParquetTableRecognizesSupportedForm(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		sql      string
		expected LocalParquetTable
	}{
		{
			name:     "create table",
			sql:      "CREATE TABLE demo.sales AS (IMPORT FROM LOCAL PARQUET FILE '/d/s.parquet')",
			expected: LocalParquetTable{Schema: "DEMO", Table: "SALES", Path: "/d/s.parquet"},
		},
		{
			name: "create or replace table",
			sql: "create or replace table Demo.Sales as " +
				"(import from local parquet file 's.parquet')",
			expected: LocalParquetTable{
				Schema: "DEMO", Table: "SALES", Path: "s.parquet", Replace: true,
			},
		},
		{
			name: "comments, line breaks and trailing semicolon",
			sql: "-- load\nCREATE /* new */ TABLE demo . sales\n  AS (\n" +
				"    IMPORT FROM LOCAL PARQUET FILE 'C:/data/s.parquet' -- local\n  );",
			expected: LocalParquetTable{Schema: "DEMO", Table: "SALES", Path: "C:/data/s.parquet"},
		},
		{
			name:     "escaped quote in path",
			sql:      "CREATE TABLE s.t AS (IMPORT FROM LOCAL PARQUET FILE '/o''brien.parquet')",
			expected: LocalParquetTable{Schema: "S", Table: "T", Path: "/o'brien.parquet"},
		},
		{
			name:     "unicode identifiers",
			sql:      "CREATE TABLE schéma.täble_1 AS (IMPORT FROM LOCAL PARQUET FILE 'f.parquet')",
			expected: LocalParquetTable{Schema: "SCHÉMA", Table: "TÄBLE_1", Path: "f.parquet"},
		},
		{
			name: "identifiers with combining mark, letter number, connector and middle dot",
			sql: "CREATE TABLE \u2160s\u00B7x.table\u0301\u203F1 AS " +
				"(IMPORT FROM LOCAL PARQUET FILE 'f')",
			expected: LocalParquetTable{
				Schema: "\u2160S\u00B7X", Table: "TABLE\u0301\u203F1", Path: "f",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// When
			table, err := ParseLocalParquetTable(test.sql)

			// Then
			require.NoError(t, err)
			require.Equal(t, &test.expected, table)
		})
	}
}

func TestParseLocalParquetTableRejectsUnsupportedShapes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		sql  string
	}{
		{
			name: "subselect",
			sql:  "CREATE TABLE s.t AS SELECT id FROM (IMPORT FROM LOCAL PARQUET FILE '/f')",
		},
		{
			name: "missing schema",
			sql:  "CREATE TABLE t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		},
		{
			name: "quoted identifiers",
			sql:  `CREATE TABLE "s"."t" AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')`,
		},
		{
			name: "identifier starting with a digit",
			sql:  "CREATE TABLE s.1t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		},
		{
			name: "identifier starting with a combining mark",
			sql:  "CREATE TABLE s.\u0301t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		},
		{
			name: "identifier longer than 128 characters",
			sql: "CREATE TABLE s." + strings.Repeat("t", 129) +
				" AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		},
		{
			name: "several files",
			sql:  "CREATE TABLE s.t AS (IMPORT FROM LOCAL PARQUET FILE '/a' FILE '/b')",
		},
		{
			name: "missing parentheses",
			sql:  "CREATE TABLE s.t AS IMPORT FROM LOCAL PARQUET FILE '/f.parquet'",
		},
		{
			name: "column list",
			sql:  "CREATE TABLE s.t (id INT) AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		},
		{
			name: "trailing clause",
			sql:  "CREATE TABLE s.t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet') WITH NO DATA",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// When
			table, err := ParseLocalParquetTable(test.sql)

			// Then
			require.ErrorIs(t, err, ErrUnsupportedLocalParquetTable)
			require.ErrorContains(t, err, LocalParquetTableForm)
			require.Nil(t, table)
		})
	}
}

func TestParseLocalParquetTableIgnoresUnrelatedStatements(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		sql  string
	}{
		{
			name: "import into existing table",
			sql:  "IMPORT INTO s.t FROM LOCAL PARQUET FILE '/f.parquet'",
		},
		{
			name: "object storage import",
			sql:  "CREATE TABLE s.t AS (IMPORT FROM PARQUET AT 'https://host/' FILE 'f.parquet')",
		},
		{
			name: "keywords inside a literal",
			sql:  "CREATE TABLE s.t AS SELECT 'LOCAL PARQUET' AS note",
		},
		{
			name: "keywords inside a comment",
			sql:  "CREATE TABLE s.t AS SELECT 1 AS x -- LOCAL PARQUET",
		},
		{
			name: "keywords inside a quoted identifier",
			sql:  `CREATE TABLE s.t AS SELECT 1 AS "LOCAL PARQUET"`,
		},
		{
			name: "create view",
			sql:  "CREATE VIEW s.v AS SELECT id FROM (IMPORT FROM LOCAL PARQUET FILE '/f.parquet')",
		},
		{
			name: "unterminated block comment",
			sql:  "CREATE TABLE s.t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet') /* oops",
		},
		{
			name: "unterminated string literal",
			sql:  "CREATE TABLE s.t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet)",
		},
		{
			name: "unterminated quoted identifier",
			sql:  `CREATE TABLE s.t AS (IMPORT FROM LOCAL PARQUET FILE '/f.parquet') "x`,
		},
		{name: "select", sql: "SELECT 1"},
		{name: "empty", sql: "   "},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// When
			table, err := ParseLocalParquetTable(test.sql)

			// Then
			require.NoError(t, err)
			require.Nil(t, table)
		})
	}
}
