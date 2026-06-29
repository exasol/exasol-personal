// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyStatement(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		sql      string
		expected StatementType
	}{
		{name: "select", sql: "SELECT 1", expected: StatementTypeSelect},
		{
			name:     "with",
			sql:      "WITH x AS (SELECT 1) SELECT * FROM x",
			expected: StatementTypeWith,
		},
		{
			name:     "merge",
			sql:      "MERGE INTO t USING s ON t.id = s.id",
			expected: StatementTypeMerge,
		},
		{
			name:     "open schema after line comment",
			sql:      "-- hi\nOPEN SCHEMA foo",
			expected: StatementTypeOpenSchema,
		},
		{
			name:     "close schema after block comment",
			sql:      "/* hi */ CLOSE SCHEMA foo",
			expected: StatementTypeCloseSchema,
		},
		{name: "set", sql: "SET AUTOCOMMIT ON", expected: StatementTypeSet},
		{name: "unknown", sql: "CALL something()", expected: StatementTypeUnknown},
		{name: "empty", sql: "   ", expected: StatementTypeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.expected, ClassifyStatement(test.sql))
		})
	}
}

func TestStatementTypeUsesExecPath(t *testing.T) {
	t.Parallel()

	require.False(t, StatementTypeSelect.UsesExecPath())
	require.False(t, StatementTypeWith.UsesExecPath())
	require.True(t, StatementTypeUpdate.UsesExecPath())
	require.True(t, StatementTypeOpenSchema.UsesExecPath())
	require.True(t, StatementTypeCloseSchema.UsesExecPath())
	require.True(t, StatementTypeMerge.UsesExecPath())
	require.False(t, StatementTypeUnknown.UsesExecPath())
}
