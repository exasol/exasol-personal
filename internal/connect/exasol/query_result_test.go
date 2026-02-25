// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"testing"

	"github.com/exasol/exasol-driver-go/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestQueryResult(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		resultSet *types.SqlQueryResponseResultSet
		expected  *QueryResult
		expectErr error
	}{
		{
			name: "single row and column",
			resultSet: &types.SqlQueryResponseResultSet{
				ResultSet: types.SqlQueryResponseResultSetData{
					NumColumns: 1,
					NumRows:    1,
					Columns: []types.SqlQueryColumn{
						{Name: "Firstname"},
					},
					Data: [][]any{{"Foo"}},
				},
			},
			expected: &QueryResult{
				columnNames: []string{"Firstname"},
				rows:        [][]string{{"Foo"}},
			},
		},
		{
			name: "multiple rows and columns",
			resultSet: &types.SqlQueryResponseResultSet{
				ResultSet: types.SqlQueryResponseResultSetData{
					NumColumns: 2,
					NumRows:    1,
					Columns: []types.SqlQueryColumn{
						{Name: "Id"},
						{Name: "Firstname"},
					},
					Data: [][]any{{1}, {"Foo"}},
				},
			},
			expected: &QueryResult{
				columnNames: []string{"Id", "Firstname"},
				rows:        [][]string{{"1", "Foo"}},
			},
		},
		{
			name: "missing value in one column",
			resultSet: &types.SqlQueryResponseResultSet{
				ResultSet: types.SqlQueryResponseResultSetData{
					NumColumns: 2,
					NumRows:    1,
					Columns: []types.SqlQueryColumn{
						{Name: "Id"},
						{Name: "Firstname"},
					},
					Data: [][]any{{1}},
				},
			},
			expectErr: ErrNumColumns,
		},
		{
			name: "missing expected number of rows",
			resultSet: &types.SqlQueryResponseResultSet{
				ResultSet: types.SqlQueryResponseResultSetData{
					NumColumns: 1,
					NumRows:    2,
					Columns: []types.SqlQueryColumn{
						{Name: "Id"},
					},
					Data: [][]any{{1}},
				},
			},
			expectErr: ErrNumRows,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			queryResult := &QueryResult{}

			err := queryResult.FromResultSet(test.resultSet)

			if test.expectErr != nil {
				require.ErrorIs(t, err, test.expectErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expected.ColumnNames(), queryResult.ColumnNames())
			require.Equal(t, test.expected.Rows(), queryResult.Rows())
		})
	}
}
