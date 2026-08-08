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
		{name: "explain", sql: "EXPLAIN SELECT 1", expected: StatementTypeExplain},
		{name: "insert", sql: "INSERT INTO t VALUES (1)", expected: StatementTypeInsert},
		{name: "delete", sql: "DELETE FROM t", expected: StatementTypeDelete},
		{name: "import", sql: "IMPORT INTO t FROM CSV", expected: StatementTypeImport},
		{name: "export", sql: "EXPORT t INTO CSV", expected: StatementTypeExport},
		{name: "create", sql: "CREATE TABLE t (id INT)", expected: StatementTypeCreate},
		{name: "alter", sql: "ALTER TABLE t ADD COLUMN x INT", expected: StatementTypeAlter},
		{name: "drop", sql: "DROP TABLE t", expected: StatementTypeDrop},
		{name: "truncate", sql: "TRUNCATE TABLE t", expected: StatementTypeTruncate},
		{name: "commit", sql: "COMMIT", expected: StatementTypeCommit},
		{name: "rollback", sql: "ROLLBACK", expected: StatementTypeRollback},
		{name: "grant", sql: "GRANT SELECT ON t TO u", expected: StatementTypeGrant},
		{name: "revoke", sql: "REVOKE SELECT ON t FROM u", expected: StatementTypeRevoke},
		{
			name:     "open without schema keyword is unknown",
			sql:      "OPEN CURSOR foo",
			expected: StatementTypeUnknown,
		},
		{
			name:     "close without schema keyword is unknown",
			sql:      "CLOSE CURSOR foo",
			expected: StatementTypeUnknown,
		},
		{name: "open alone is unknown", sql: "OPEN", expected: StatementTypeUnknown},
		{name: "close alone is unknown", sql: "CLOSE", expected: StatementTypeUnknown},
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
	require.False(t, StatementTypeExplain.UsesExecPath())
	require.True(t, StatementTypeInsert.UsesExecPath())
	require.True(t, StatementTypeUpdate.UsesExecPath())
	require.True(t, StatementTypeDelete.UsesExecPath())
	require.True(t, StatementTypeMerge.UsesExecPath())
	require.True(t, StatementTypeImport.UsesExecPath())
	require.True(t, StatementTypeExport.UsesExecPath())
	require.True(t, StatementTypeCreate.UsesExecPath())
	require.True(t, StatementTypeAlter.UsesExecPath())
	require.True(t, StatementTypeDrop.UsesExecPath())
	require.True(t, StatementTypeTruncate.UsesExecPath())
	require.True(t, StatementTypeOpenSchema.UsesExecPath())
	require.True(t, StatementTypeCloseSchema.UsesExecPath())
	require.True(t, StatementTypeSet.UsesExecPath())
	require.True(t, StatementTypeCommit.UsesExecPath())
	require.True(t, StatementTypeRollback.UsesExecPath())
	require.True(t, StatementTypeGrant.UsesExecPath())
	require.True(t, StatementTypeRevoke.UsesExecPath())
	require.False(t, StatementTypeUnknown.UsesExecPath())
	require.False(t, StatementType("NOT_A_REAL_TYPE").UsesExecPath())
}
