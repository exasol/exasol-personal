// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeStructuredSQLError struct {
	structured StructuredSQLError
}

func (fakeStructuredSQLError) Error() string {
	return "wrapped structured sql error"
}

func (f fakeStructuredSQLError) StructuredSQLError() StructuredSQLError {
	return f.structured
}

func TestStructuredSQLErrorFromError(t *testing.T) {
	t.Parallel()

	t.Run("parses real driver format and falls back errorCode to sql state", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(errors.New(
			"E-EGOD-11: execution failed with SQL error code '42000' and message " +
				"'syntax error near SELECT'",
		))

		require.Equal(t, "42000", structured.ErrorCode)
		require.Equal(t, "42000", structured.SQLState)
		require.Equal(t, "syntax error near SELECT", structured.Message)
		require.Nil(t, structured.SessionID)
		require.Nil(t, structured.Position)
	})

	t.Run("prefers vendor error code from message when present", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(errors.New(
			"E-EGOD-11: execution failed with SQL error code '42636' and message " +
				"'ETL-6009: syntax error near SELECT at line 3, column 7'",
		))

		require.Equal(t, "ETL-6009", structured.ErrorCode)
		require.Equal(t, "42636", structured.SQLState)
		require.Equal(
			t,
			"ETL-6009: syntax error near SELECT at line 3, column 7",
			structured.Message,
		)
		require.NotNil(t, structured.Position)
		require.Equal(t, 3, *structured.Position.Line)
		require.Equal(t, 7, *structured.Position.Column)
	})

	t.Run("captures whichever position fields are available", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(errors.New(
			"E-EGOD-11: execution failed with SQL error code '42636' and message " +
				"'syntax error near SELECT at column 7'",
		))

		require.Equal(t, "42636", structured.ErrorCode)
		require.Equal(t, "42636", structured.SQLState)
		require.NotNil(t, structured.Position)
		require.Nil(t, structured.Position.Line)
		require.Equal(t, 7, *structured.Position.Column)
	})

	t.Run("ignores non-positive position values", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(errors.New(
			"E-EGOD-11: execution failed with SQL error code '42636' and message " +
				"'syntax error near SELECT at line 0, column 7'",
		))

		require.Equal(t, "42636", structured.ErrorCode)
		require.Equal(t, "42636", structured.SQLState)
		require.NotNil(t, structured.Position)
		require.Nil(t, structured.Position.Line)
		require.Equal(t, 7, *structured.Position.Column)
	})

	t.Run("falls back to unknown for non driver errors", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(errors.New("plain network failure"))

		require.Equal(t, "UNKNOWN", structured.ErrorCode)
		require.Empty(t, structured.SQLState)
		require.Equal(t, "plain network failure", structured.Message)
		require.Nil(t, structured.SessionID)
		require.Nil(t, structured.Position)
	})

	t.Run("parses wrapped driver errors", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(
			fmt.Errorf(
				"retrying execution: %w",
				errors.New(
					"E-EGOD-11: execution failed with SQL error code '22003' and message "+
						"'numeric value out of range'",
				),
			),
		)

		require.Equal(t, "22003", structured.ErrorCode)
		require.Equal(t, "22003", structured.SQLState)
		require.Equal(t, "numeric value out of range", structured.Message)
	})

	t.Run("parses prefixed multiline driver errors", func(t *testing.T) {
		t.Parallel()

		structured := StructuredSQLErrorFromError(errors.New(
			"failed to login: E-EGOD-11: execution failed with SQL error code '42636' and " +
				"message 'ETL-6009: syntax error near SELECT\nat line 3, column 7'",
		))

		require.Equal(t, "ETL-6009", structured.ErrorCode)
		require.Equal(t, "42636", structured.SQLState)
		require.Equal(
			t,
			"ETL-6009: syntax error near SELECT\nat line 3, column 7",
			structured.Message,
		)
		require.NotNil(t, structured.Position)
		require.Equal(t, 3, *structured.Position.Line)
		require.Equal(t, 7, *structured.Position.Column)
	})

	t.Run("prefers a pre-structured carrier", func(t *testing.T) {
		t.Parallel()

		expected := StructuredSQLError{
			ErrorCode: "ETL-6009",
			SQLState:  "42636",
			Message:   "structured already",
			SessionID: ptrToString("12345"),
		}

		structured := StructuredSQLErrorFromError(fakeStructuredSQLError{structured: expected})

		require.Equal(t, expected, structured)
	})
}

func ptrToString(value string) *string {
	return &value
}
