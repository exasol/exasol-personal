// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/connect/exasol/types"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types/typesfakes"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("error")

// fakeRows is a minimal driver.Rows test double that serves predefined rows
// one Next call at a time and records how many times Close was called.
type fakeRows struct {
	columns    []string
	data       [][]driver.Value
	pos        int
	closeCount int
}

func (f *fakeRows) Columns() []string { return f.columns }

func (f *fakeRows) Close() error {
	f.closeCount++

	return nil
}

func (f *fakeRows) Next(dest []driver.Value) error {
	if f.pos >= len(f.data) {
		return io.EOF
	}

	copy(dest, f.data[f.pos])
	f.pos++

	return nil
}

func testDatabaseFactory(t *testing.T, connect types.ConnectFunc) generaltypes.Databaser {
	t.Helper()

	database, err := New(
		"foo",
		"bar",
		"192.168.0.1",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		8563,
		WithConnectFunc(connect))
	require.NoError(t, err)

	return database
}

func TestConnect(t *testing.T) {
	t.Parallel()

	type mocks struct {
		fakeConnector *typesfakes.FakeExasolConnector
	}

	for _, test := range []struct {
		name  string
		given func(*mocks)
		then  func(*testing.T, *mocks, error)
	}{
		{
			name: "database successfully connects",
			given: func(mocks *mocks) {
				mocks.fakeConnector.QueryContextReturns(&fakeRows{
					columns: []string{"v"},
					data:    [][]driver.Value{{"2025.1.0"}},
				}, nil)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.QueryContextCallCount())
			},
		},
		{
			name: "version query fails",
			given: func(mocks *mocks) {
				mocks.fakeConnector.QueryContextReturns(nil, errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()
				// A failing version query is tolerated (cosmetic only).
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.QueryContextCallCount())
			},
		},
		{
			name: "version query returns empty result",
			given: func(mocks *mocks) {
				mocks.fakeConnector.QueryContextReturns(&fakeRows{columns: []string{}}, nil)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.QueryContextCallCount())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mocks := &mocks{&typesfakes.FakeExasolConnector{}}
			test.given(mocks)

			connect := func(string) (types.ExasolConnector, error) {
				return mocks.fakeConnector, nil
			}

			database := testDatabaseFactory(t, connect)

			err := database.Connect(t.Context())
			test.then(t, mocks, err)

			err = database.Close()
			require.NoError(t, err)
		})
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	fakeConnector := &typesfakes.FakeExasolConnector{}
	connect := func(string) (types.ExasolConnector, error) {
		return fakeConnector, nil
	}

	database := testDatabaseFactory(t, connect)

	require.PanicsWithValue(t, closePanicMsg, func() {
		database.Close() // nolint: gosec
	})
}

func TestExec(t *testing.T) {
	t.Parallel()

	type mocks struct {
		fakeConnector *typesfakes.FakeExasolConnector
	}

	for _, test := range []struct {
		name      string
		query     string
		maxRows   int
		queryRows driver.Rows
		queryErr  error
		execErr   error
		then      func(*testing.T, *mocks, generaltypes.QueryResulter, error)
	}{
		{
			name:  "select query",
			query: "SELECT * FROM Dual",
			queryRows: &fakeRows{
				columns: []string{"col1", "col2"},
				data:    [][]driver.Value{{"val1", "val2"}},
			},
			then: func(t *testing.T, _ *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, []string{"col1", "col2"}, result.ColumnNames())
				require.Equal(t, [][]string{{"val1", "val2"}}, result.Rows())
				require.Equal(t, [][]any{{"val1", "val2"}}, result.Values())
				require.False(t, result.Truncated())
			},
		},
		{
			name:     "select query error",
			query:    "SELECT * FROM Dual",
			queryErr: errTest,
			then: func(t *testing.T, _ *mocks, _ generaltypes.QueryResulter, err error) {
				t.Helper()
				require.ErrorIs(t, err, errTest)
			},
		},
		{
			name:      "non-resultset statement",
			query:     "OPEN SCHEMA dummy",
			queryRows: &fakeRows{columns: []string{}},
			then: func(t *testing.T, _ *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Empty(t, result.ColumnNames())
				require.Empty(t, result.Rows())
				require.Empty(t, result.Values())
				require.False(t, result.Truncated())
			},
		},
		{
			name:  "file import query",
			query: "IMPORT INTO dummy FROM LOCAL CSV FILE './dummy.csv'",
			then: func(t *testing.T, mocks *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.ExecCallCount())
				require.Equal(t, []string{}, result.ColumnNames())
				require.Equal(t, [][]string{}, result.Rows())
				require.Equal(t, [][]any{}, result.Values())
			},
		},
		{
			name:    "file import query error",
			query:   "IMPORT INTO dummy FROM LOCAL CSV FILE './dummy.csv'",
			execErr: errTest,
			then: func(t *testing.T, mocks *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.ErrorIs(t, err, errTest)
				require.Equal(t, 1, mocks.fakeConnector.ExecCallCount())
				require.Equal(t, []string{}, result.ColumnNames())
				require.Equal(t, [][]string{}, result.Rows())
				require.Equal(t, [][]any{}, result.Values())
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mocks := &mocks{&typesfakes.FakeExasolConnector{}}

			// version() (called by Connect) also goes through QueryContext, so
			// answer the version query separately and reserve the per-case rows
			// for the query under test.
			mocks.fakeConnector.QueryContextStub = func(
				_ context.Context, query string, _ []driver.NamedValue,
			) (driver.Rows, error) {
				if strings.Contains(query, "exa_metadata") {
					return &fakeRows{
						columns: []string{"v"},
						data:    [][]driver.Value{{"2025.1.0"}},
					}, nil
				}

				return test.queryRows, test.queryErr
			}
			mocks.fakeConnector.ExecReturns(nil, test.execErr)

			connect := func(string) (types.ExasolConnector, error) {
				return mocks.fakeConnector, nil
			}

			database := testDatabaseFactory(t, connect)
			require.NoError(t, database.Connect(t.Context()))

			result, err := database.Exec(t.Context(), test.query, test.maxRows)

			test.then(t, mocks, result, err)

			err = database.Close()
			require.NoError(t, err)
		})
	}
}

func TestExecClosesResultRows(t *testing.T) {
	t.Parallel()

	queryRows := &fakeRows{
		columns: []string{"n"},
		data:    [][]driver.Value{{int64(1)}, {int64(2)}},
	}

	fakeConnector := &typesfakes.FakeExasolConnector{}
	fakeConnector.QueryContextStub = func(
		_ context.Context, query string, _ []driver.NamedValue,
	) (driver.Rows, error) {
		if strings.Contains(query, "exa_metadata") {
			return &fakeRows{columns: []string{"v"}, data: [][]driver.Value{{"2025.1.0"}}}, nil
		}

		return queryRows, nil
	}

	database := testDatabaseFactory(t, func(string) (types.ExasolConnector, error) {
		return fakeConnector, nil
	})
	require.NoError(t, database.Connect(t.Context()))

	_, err := database.Exec(t.Context(), "SELECT * FROM Dual", 0)
	require.NoError(t, err)

	// The handle is closed exactly once after consumption.
	require.Equal(t, 1, queryRows.closeCount)
}

func TestCollectRows(t *testing.T) {
	t.Parallel()

	makeRows := func(n int) *fakeRows {
		data := make([][]driver.Value, 0, n)
		for i := range n {
			data = append(data, []driver.Value{int64(i)})
		}

		return &fakeRows{columns: []string{"n"}, data: data}
	}

	t.Run("unlimited returns every row", func(t *testing.T) {
		t.Parallel()

		rows := makeRows(5)
		result, err := collectRows(rows, 0)

		require.NoError(t, err)
		require.Len(t, result.Rows(), 5)
		require.False(t, result.Truncated())
		require.Equal(t, 5, rows.pos) // all consumed, no extra read
	})

	t.Run("limit truncates and stops fetching", func(t *testing.T) {
		t.Parallel()

		rows := makeRows(5)
		result, err := collectRows(rows, 2)

		require.NoError(t, err)
		require.Len(t, result.Rows(), 2)
		require.True(t, result.Truncated())
		require.Equal(t, 3, rows.pos) // 2 collected + 1 peek, nothing more
	})

	t.Run("limit equal to row count is not truncated", func(t *testing.T) {
		t.Parallel()

		rows := makeRows(2)
		result, err := collectRows(rows, 2)

		require.NoError(t, err)
		require.Len(t, result.Rows(), 2)
		require.False(t, result.Truncated())
	})

	t.Run("integer DECIMAL values render without float artifacts", func(t *testing.T) {
		t.Parallel()

		rows := &fakeRows{columns: []string{"n"}, data: [][]driver.Value{{int64(1000000)}}}
		result, err := collectRows(rows, 0)

		require.NoError(t, err)
		require.Equal(t, [][]string{{"1000000"}}, result.Rows())
		require.Equal(t, [][]any{{int64(1000000)}}, result.Values())
	})

	t.Run("preserves json-compatible values beside display strings", func(t *testing.T) {
		t.Parallel()

		timestamp := time.Date(2026, 6, 23, 12, 30, 45, 123000000, time.UTC)
		rows := &fakeRows{
			columns: []string{"n", "fraction", "ok", "missing", "text", "payload", "created_at"},
			data: [][]driver.Value{{
				int64(42),
				float64(1.5),
				true,
				nil,
				"<tag>&value",
				[]byte("bytes"),
				timestamp,
			}},
		}

		result, err := collectRows(rows, 0)

		require.NoError(t, err)
		require.Equal(t, [][]string{{
			"42",
			"1.5",
			"true",
			"<nil>",
			"<tag>&value",
			"[98 121 116 101 115]",
			"2026-06-23 12:30:45.123 +0000 UTC",
		}}, result.Rows())
		require.Equal(t, [][]any{{
			int64(42),
			float64(1.5),
			true,
			nil,
			"<tag>&value",
			"bytes",
			"2026-06-23T12:30:45.123Z",
		}}, result.Values())
	})

	t.Run("propagates a non-EOF Next error", func(t *testing.T) {
		t.Parallel()

		_, err := collectRows(&errRows{err: errTest}, 0)
		require.ErrorIs(t, err, errTest)
	})
}

// errRows is a driver.Rows whose Next always fails.
type errRows struct {
	err error
}

func (*errRows) Columns() []string { return []string{"n"} }
func (*errRows) Close() error      { return nil }
func (e *errRows) Next([]driver.Value) error {
	return e.err
}
