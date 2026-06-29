// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/stretchr/testify/require"
)

var (
	prettyJSONResult = "{\n" +
		"  \"columns\": [\n" +
		"    \"ID\",\n" +
		"    \"NAME\"\n" +
		"  ],\n" +
		"  \"rows\": [\n" +
		"    [\n" +
		"      \"1\",\n" +
		"      \"Alice\"\n" +
		"    ],\n" +
		"    [\n" +
		"      \"2\",\n" +
		"      \"Bob\"\n" +
		"    ]\n" +
		"  ]\n" +
		"}\n"
	compactJSONResult = "{\"columns\":[\"ID\",\"NAME\"]," +
		"\"rows\":[[\"1\",\"Alice\"],[\"2\",\"Bob\"]]}\n"
	emptyPrettyJSON = "{\n" +
		"  \"columns\": [],\n" +
		"  \"rows\": []\n" +
		"}\n"
	unknownPrettyJSON = "{\n" +
		"  \"columns\": [\n" +
		"    \"ID\"\n" +
		"  ],\n" +
		"  \"rows\": [\n" +
		"    [\n" +
		"      \"1\"\n" +
		"    ]\n" +
		"  ]\n" +
		"}\n"
)

type stubQueryResult struct {
	columnNames   []string
	rows          [][]string
	values        [][]any
	statementType generaltypes.StatementType
	rowsAffected  int64
	truncated     bool
}

func (s stubQueryResult) ColumnNames() []string {
	return s.columnNames
}

func (s stubQueryResult) Rows() [][]string {
	return s.rows
}

func (s stubQueryResult) Values() [][]any {
	if s.values != nil {
		return s.values
	}

	values := make([][]any, len(s.rows))
	for i, row := range s.rows {
		values[i] = make([]any, len(row))
		for j, value := range row {
			values[i][j] = value
		}
	}

	return values
}

func (s stubQueryResult) StatementType() generaltypes.StatementType {
	if s.statementType == "" {
		return generaltypes.StatementTypeUnknown
	}

	return s.statementType
}

func (s stubQueryResult) RowsAffected() int64 {
	return s.rowsAffected
}

func (s stubQueryResult) Truncated() bool {
	return s.truncated
}

func TestPrintResultJSON(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		format     JSONFormat
		result     stubQueryResult
		expected   string
		normalized JSONFormat
	}{
		{
			name:   "renders columns and rows as pretty json",
			format: JSONFormatPretty,
			result: stubQueryResult{
				columnNames: []string{"ID", "NAME"},
				rows:        [][]string{{"1", "Alice"}, {"2", "Bob"}},
			},
			expected:   prettyJSONResult,
			normalized: JSONFormatPretty,
		},
		{
			name:   "renders columns and rows as compact json",
			format: JSONFormatCompact,
			result: stubQueryResult{
				columnNames: []string{"ID", "NAME"},
				rows:        [][]string{{"1", "Alice"}, {"2", "Bob"}},
			},
			expected:   compactJSONResult,
			normalized: JSONFormatCompact,
		},
		{
			name:   "renders empty results as empty arrays",
			format: JSONFormatPretty,
			result: stubQueryResult{
				columnNames: []string{},
				rows:        [][]string{},
			},
			expected:   emptyPrettyJSON,
			normalized: JSONFormatPretty,
		},
		{
			name:   "unknown format falls back to pretty json",
			format: JSONFormat("surprise"),
			result: stubQueryResult{
				columnNames: []string{"ID"},
				rows:        [][]string{{"1"}},
			},
			expected:   unknownPrettyJSON,
			normalized: JSONFormatPretty,
		},
		{
			name:   "renders typed values without html escaping",
			format: JSONFormatCompact,
			result: stubQueryResult{
				columnNames: []string{"N", "OK", "MISSING", "TEXT", "CREATED_AT"},
				rows: [][]string{{
					"42",
					"true",
					"<nil>",
					"<tag>&value",
					"2026-06-23T12:00:00Z",
				}},
				values: [][]any{{
					int64(42),
					true,
					nil,
					"<tag>&value",
					"2026-06-23T12:00:00Z",
				}},
			},
			expected: "{\"columns\":[\"N\",\"OK\",\"MISSING\",\"TEXT\",\"CREATED_AT\"]," +
				"\"rows\":[[42,true,null,\"<tag>&value\",\"2026-06-23T12:00:00Z\"]]}\n",
			normalized: JSONFormatCompact,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			// Given a query result that should be emitted as JSON.
			// When the result is rendered in the requested JSON mode.
			printer := newJSONResultPrinter(test.format)
			err := printer(&buf, test.result)

			// Then the output is valid JSON in the expected shape.
			require.NoError(t, err)
			require.Equal(t, test.expected, buf.String())
			require.Equal(t, test.normalized, normalizeJSONFormat(test.format))

			var decoded jsonQueryResult
			require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
			require.Equal(t, test.result.columnNames, decoded.Columns)
			require.Equal(t, jsonRoundTripValues(t, test.result.Values()), decoded.Rows)
		})
	}
}

func TestPrintResultCSV(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		result   stubQueryResult
		expected string
	}{
		{
			name: "renders header and rows",
			result: stubQueryResult{
				columnNames: []string{"ID", "NAME"},
				values:      [][]any{{int64(1), "Alice"}, {int64(2), "Bob"}},
			},
			expected: "ID,NAME\n1,Alice\n2,Bob\n",
		},
		{
			name: "quotes csv special characters",
			result: stubQueryResult{
				columnNames: []string{"ID", "NOTE"},
				values: [][]any{{
					int64(1),
					"Alice, \"A\"\nLine",
				}},
			},
			expected: "ID,NOTE\n1,\"Alice, \"\"A\"\"\nLine\"\n",
		},
		{
			name: "renders null as empty field",
			result: stubQueryResult{
				columnNames: []string{"ID", "MISSING", "NAME"},
				rows:        [][]string{{"1", "<nil>", "Alice"}},
				values:      [][]any{{int64(1), nil, "Alice"}},
			},
			expected: "ID,MISSING,NAME\n1,,Alice\n",
		},
		{
			name: "skips zero-column results",
			result: stubQueryResult{
				columnNames: []string{},
				values:      [][]any{},
			},
			expected: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			// Given a query result that should be emitted as CSV.
			// When the result is rendered in CSV mode.
			err := printResultCSV(&buf, test.result)

			// Then the output follows standard CSV encoding.
			require.NoError(t, err)
			require.Equal(t, test.expected, buf.String())
		})
	}
}

func jsonRoundTripValues(t *testing.T, values [][]any) [][]any {
	t.Helper()

	data, err := json.Marshal(values)
	require.NoError(t, err)

	var decoded [][]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	return decoded
}

func TestResolveNonInteractiveSQL(t *testing.T) {
	t.Parallel()

	t.Run("command supplies SQL", func(t *testing.T) {
		t.Parallel()

		sql, nonInteractive, err := resolveNonInteractiveSQLFrom(
			&Opts{Command: "SELECT 1"},
			bytes.NewBuffer(nil),
			true,
		)

		require.NoError(t, err)
		require.True(t, nonInteractive)
		require.Equal(t, "SELECT 1", sql)
	})

	t.Run("file supplies SQL", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "script.sql")
		require.NoError(t, os.WriteFile(path, []byte("SELECT 1; SELECT 2;"), 0o600))

		sql, nonInteractive, err := resolveNonInteractiveSQLFrom(
			&Opts{File: path},
			bytes.NewBuffer(nil),
			true,
		)

		require.NoError(t, err)
		require.True(t, nonInteractive)
		require.Equal(t, "SELECT 1; SELECT 2;", sql)
	})

	t.Run("missing file fails without running statements", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "does-not-exist.sql")

		sql, nonInteractive, err := resolveNonInteractiveSQLFrom(
			&Opts{File: path},
			bytes.NewBuffer(nil),
			true,
		)

		require.Error(t, err)
		require.ErrorContains(t, err, "reading SQL file")
		require.False(t, nonInteractive)
		require.Empty(t, sql)
	})

	t.Run("no flag falls back to interactive", func(t *testing.T) {
		t.Parallel()

		sql, nonInteractive, err := resolveNonInteractiveSQLFrom(
			&Opts{},
			bytes.NewBuffer(nil),
			true,
		)

		require.NoError(t, err)
		require.False(t, nonInteractive)
		require.Empty(t, sql)
	})

	t.Run("piped stdin supplies SQL non-interactively for json output", func(t *testing.T) {
		t.Parallel()

		sql, nonInteractive, err := resolveNonInteractiveSQLFrom(
			&Opts{OutputFormat: OutputFormatJSON},
			bytes.NewBufferString("SELECT 1; SELECT 2;"),
			false,
		)

		require.NoError(t, err)
		require.True(t, nonInteractive)
		require.Equal(t, "SELECT 1; SELECT 2;", sql)
	})

	t.Run("piped stdin keeps shell path for non json output", func(t *testing.T) {
		t.Parallel()

		sql, nonInteractive, err := resolveNonInteractiveSQLFrom(
			&Opts{OutputFormat: OutputFormatTable},
			bytes.NewBufferString("SELECT 1; SELECT 2;"),
			false,
		)

		require.NoError(t, err)
		require.False(t, nonInteractive)
		require.Empty(t, sql)
	})
}

type stubDatabase struct {
	results []stubExecResult
	queries []string
	maxRows []int
}

type stubExecResult struct {
	result stubQueryResult
	err    error
}

func (*stubDatabase) Connect(context.Context) error { return nil }
func (*stubDatabase) Close() error                  { return nil }

func (db *stubDatabase) Exec(
	_ context.Context,
	query string,
	maxRows int,
) (generaltypes.QueryResulter, error) {
	db.queries = append(db.queries, query)
	db.maxRows = append(db.maxRows, maxRows)

	if len(db.results) == 0 {
		return nil, errors.New("unexpected Exec call")
	}

	next := db.results[0]
	db.results = db.results[1:]

	if next.err != nil {
		return nil, next.err
	}

	return next.result, nil
}

func TestRunJSONStatements(t *testing.T) {
	t.Parallel()

	t.Run("single statement renders one invocation envelope", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		database := &stubDatabase{
			results: []stubExecResult{{
				result: stubQueryResult{
					columnNames:   []string{"N"},
					values:        [][]any{{int64(1)}},
					statementType: generaltypes.StatementTypeSelect,
				},
			}},
		}

		err := runJSONStatements(t.Context(), "SELECT 1", database, &buf, JSONFormatCompact, 0)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"statements": [
				{
					"statement": "SELECT 1",
					"statementType": "SELECT",
					"rowsAffected": 0,
					"columns": ["N"],
					"rows": [[1]],
					"truncated": false
				}
			]
		}`, buf.String())
	})

	t.Run("multi statement output includes ddl metadata", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		database := &stubDatabase{
			results: []stubExecResult{
				{
					result: stubQueryResult{
						columnNames:   []string{},
						values:        [][]any{},
						statementType: generaltypes.StatementTypeOpenSchema,
						rowsAffected:  0,
					},
				},
				{
					result: stubQueryResult{
						columnNames:   []string{"N"},
						values:        [][]any{},
						statementType: generaltypes.StatementTypeSelect,
						rowsAffected:  0,
					},
				},
			},
		}

		err := runJSONStatements(
			t.Context(),
			"OPEN SCHEMA foo; SELECT 1 WHERE FALSE",
			database,
			&buf,
			JSONFormatCompact,
			0,
		)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"statements": [
				{
					"statement": "OPEN SCHEMA foo",
					"statementType": "OPEN_SCHEMA",
					"rowsAffected": 0,
					"columns": [],
					"rows": [],
					"truncated": false
				},
				{
					"statement": "SELECT 1 WHERE FALSE",
					"statementType": "SELECT",
					"rowsAffected": 0,
					"columns": ["N"],
					"rows": [],
					"truncated": false
				}
			]
		}`, buf.String())
	})

	t.Run("failing statement is rendered as structured json error", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		database := &stubDatabase{
			results: []stubExecResult{
				{
					result: stubQueryResult{
						columnNames:   []string{"N"},
						values:        [][]any{{int64(1)}},
						statementType: generaltypes.StatementTypeSelect,
					},
				},
				{
					err: errors.New(
						"E-EGOD-11: execution failed with SQL error code '42636' and message " +
							"'ETL-6009: syntax error near SELECT at line 3, " +
							"column 7 session 12345'",
					),
				},
			},
		}

		err := runJSONStatements(
			t.Context(),
			"SELECT 1; INVALID SQL",
			database,
			&buf,
			JSONFormatCompact,
			0,
		)

		require.Error(t, err)
		var silentErr interface{ SuppressCLIError() bool }
		require.ErrorAs(t, err, &silentErr)
		require.True(t, silentErr.SuppressCLIError())
		var decoded jsonExecutionResult
		require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
		require.Len(t, decoded.Statements, 2)
		require.Equal(t, "SELECT 1", decoded.Statements[0].Statement)
		require.Equal(t, generaltypes.StatementTypeSelect, decoded.Statements[0].StatementType)
		require.Equal(t, [][]any{{float64(1)}}, decoded.Statements[0].Rows)

		require.Equal(t, "INVALID SQL", decoded.Statements[1].Statement)
		require.Equal(t, generaltypes.StatementTypeUnknown, decoded.Statements[1].StatementType)
		require.NotNil(t, decoded.Statements[1].Error)
		require.Equal(t, "ETL-6009", decoded.Statements[1].Error.ErrorCode)
		require.Equal(t, "42636", decoded.Statements[1].Error.SQLState)
		require.Equal(
			t,
			"ETL-6009: syntax error near SELECT at line 3, column 7 session 12345",
			decoded.Statements[1].Error.Message,
		)
		require.NotNil(t, decoded.Statements[1].Error.SessionID)
		require.Equal(t, "12345", *decoded.Statements[1].Error.SessionID)
		require.NotNil(t, decoded.Statements[1].Error.Position)
		require.Equal(t, 3, *decoded.Statements[1].Error.Position.Line)
		require.Equal(t, 7, *decoded.Statements[1].Error.Position.Column)
		require.Equal(t, []string{"SELECT 1", "INVALID SQL"}, database.queries)
	})
}

func TestEffectiveMaxRows(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		requested   int
		modeDefault int
		expected    int
	}{
		{name: "unset, interactive", requested: MaxRowsUnset, modeDefault: 100, expected: 100},
		{name: "unset, unlimited", requested: MaxRowsUnset, modeDefault: 0, expected: 0},
		{name: "explicit over interactive", requested: 5, modeDefault: 100, expected: 5},
		{name: "explicit over unlimited", requested: 5, modeDefault: 0, expected: 5},
		{name: "explicit zero is unlimited", requested: 0, modeDefault: 100, expected: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, effectiveMaxRows(test.requested, test.modeDefault))
		})
	}
}

func TestPrintTruncationFooter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	require.NoError(t, printTruncationFooter(&buf, 100))

	require.Equal(t,
		"-- showing first 100 rows (output truncated; use --max-rows 0 to see all)\n",
		buf.String(),
	)
}

func TestParseJSONFormat(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		input     string
		expected  JSONFormat
		wantError bool
	}{
		{name: "empty defaults to pretty", input: "", expected: JSONFormatPretty},
		{name: "pretty accepted", input: "pretty", expected: JSONFormatPretty},
		{name: "compact accepted", input: "compact", expected: JSONFormatCompact},
		{name: "case and whitespace normalized", input: "  PRETTY  ", expected: JSONFormatPretty},
		{name: "invalid value rejected", input: "yaml", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			format, err := ParseJSONFormat(test.input)
			if test.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected, format)
		})
	}
}
