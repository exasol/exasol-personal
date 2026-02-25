// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"encoding/json"
	"errors"
	"testing"

	exasoltypes "github.com/exasol/exasol-driver-go/pkg/types"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types/typesfakes"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("error")

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

	versionResult := &exasoltypes.SqlQueryResponseResultSet{
		ResultSet: exasoltypes.SqlQueryResponseResultSetData{
			NumColumns: 1,
			NumRows:    1,
			Columns:    []exasoltypes.SqlQueryColumn{{Name: "v"}},
			Data:       [][]any{{"2025.1.0"}},
		},
	}

	versionResultJSON, err := json.Marshal(versionResult)
	require.NoError(t, err)

	versionQueryResponse := &exasoltypes.SqlQueriesResponse{
		NumResults: 1,
		Results:    []json.RawMessage{versionResultJSON},
	}

	for _, test := range []struct {
		name  string
		given func(*mocks)
		then  func(*testing.T, *mocks, error)
	}{
		{
			name: "database successfully connects",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(versionQueryResponse, nil)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.SimpleExecCallCount())
			},
		},
		{
			name: "version query fails",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(nil, errTest)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.SimpleExecCallCount())
			},
		},
		{
			name: "version query returns empty result",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(&exasoltypes.SqlQueriesResponse{
					NumResults: 0,
					Results:    []json.RawMessage{},
				}, nil)
			},
			then: func(t *testing.T, mocks *mocks, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.SimpleExecCallCount())
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

	dummyResult := &exasoltypes.SqlQueryResponseResultSet{
		ResultSet: exasoltypes.SqlQueryResponseResultSetData{
			NumColumns: 2,
			NumRows:    1,
			Columns:    []exasoltypes.SqlQueryColumn{{Name: "col1"}, {Name: "col2"}},
			Data:       [][]any{{"val1"}, {"val2"}},
		},
	}

	dummyResultJSON, err := json.Marshal(dummyResult)
	require.NoError(t, err)

	dummyQueryResponse := &exasoltypes.SqlQueriesResponse{
		NumResults: 1,
		Results:    []json.RawMessage{dummyResultJSON},
	}

	for _, test := range []struct {
		name  string
		query string
		given func(*mocks)
		then  func(*testing.T, *mocks, generaltypes.QueryResulter, error)
	}{
		{
			name:  "select query",
			query: "SELECT * FROM Dual",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(dummyQueryResponse, nil)
			},
			then: func(t *testing.T, mocks *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 2, mocks.fakeConnector.SimpleExecCallCount())
				require.Equal(t, []string{"col1", "col2"}, result.ColumnNames())
				require.Equal(t, [][]string{{"val1", "val2"}}, result.Rows())
			},
		},
		{
			name:  "select query error",
			query: "SELECT * FROM Dual",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(nil, errTest)
			},
			then: func(t *testing.T, mocks *mocks, _ generaltypes.QueryResulter, err error) {
				t.Helper()
				require.ErrorIs(t, err, errTest)
				require.Equal(t, 2, mocks.fakeConnector.SimpleExecCallCount())
			},
		},
		{
			name:  "empty result",
			query: "SELECT * FROM Dual",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(&exasoltypes.SqlQueriesResponse{
					NumResults: 0,
				}, nil)
			},
			then: func(t *testing.T, mocks *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 2, mocks.fakeConnector.SimpleExecCallCount())
				require.Empty(t, result.ColumnNames())
				require.Empty(t, result.Rows())
			},
		},
		{
			name:  "invalid result",
			query: "SELECT * FROM Dual",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(&exasoltypes.SqlQueriesResponse{
					NumResults: 0,
					Results:    []json.RawMessage{json.RawMessage("bad_result")},
				}, nil)
			},
			then: func(t *testing.T, mocks *mocks, _ generaltypes.QueryResulter, err error) {
				t.Helper()
				require.Error(t, err)
				require.Equal(t, 2, mocks.fakeConnector.SimpleExecCallCount())
			},
		},
		{
			name:  "file import query",
			query: "IMPORT INTO dummy FROM LOCAL CSV FILE './dummy.csv'",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(dummyQueryResponse, nil)
				mocks.fakeConnector.ExecReturns(nil, nil)
			},
			then: func(t *testing.T, mocks *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, 1, mocks.fakeConnector.SimpleExecCallCount())
				require.Equal(t, 1, mocks.fakeConnector.ExecCallCount())
				require.Equal(t, []string{}, result.ColumnNames())
				require.Equal(t, [][]string{}, result.Rows())
			},
		},
		{
			name:  "file import query error",
			query: "IMPORT INTO dummy FROM LOCAL CSV FILE './dummy.csv'",
			given: func(mocks *mocks) {
				mocks.fakeConnector.SimpleExecReturns(dummyQueryResponse, nil)
				mocks.fakeConnector.ExecReturns(nil, errTest)
			},
			then: func(t *testing.T, mocks *mocks, result generaltypes.QueryResulter, err error) {
				t.Helper()
				require.ErrorIs(t, err, errTest)
				require.Equal(t, 1, mocks.fakeConnector.SimpleExecCallCount())
				require.Equal(t, 1, mocks.fakeConnector.ExecCallCount())
				require.Equal(t, []string{}, result.ColumnNames())
				require.Equal(t, [][]string{}, result.Rows())
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

			require.NoError(t, database.Connect(t.Context()))

			result, err := database.Exec(t.Context(), test.query)

			test.then(t, mocks, result, err)

			err = database.Close()
			require.NoError(t, err)
		})
	}
}
