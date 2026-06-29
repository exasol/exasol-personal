// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"context"
	"encoding/json"
	"io"
	"os"

	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

type jsonStatementResult struct {
	Statement     string                           `json:"statement"`
	StatementType generaltypes.StatementType       `json:"statementType"`
	RowsAffected  int64                            `json:"rowsAffected"`
	Columns       []string                         `json:"columns"`
	Rows          [][]any                          `json:"rows"`
	Truncated     bool                             `json:"truncated"`
	Error         *generaltypes.StructuredSQLError `json:"error,omitempty"`
}

type jsonExecutionResult struct {
	Statements []jsonStatementResult `json:"statements"`
}

type renderedJSONSQLError struct {
	cause error
}

func (e renderedJSONSQLError) Error() string {
	return e.cause.Error()
}

func (e renderedJSONSQLError) Unwrap() error {
	return e.cause
}

func (renderedJSONSQLError) SuppressCLIError() bool {
	return true
}

func runJSONStatements(
	ctx context.Context,
	sql string,
	database generaltypes.Databaser,
	output io.Writer,
	jsonFormat JSONFormat,
	maxRows int,
) error {
	statements := nonInteractiveStatements(sql)
	document := jsonExecutionResult{
		Statements: make([]jsonStatementResult, 0, len(statements)),
	}

	var executionErr error
	for _, statement := range statements {
		queryResult, err := database.Exec(ctx, statement, maxRows)
		if err != nil {
			structuredErr := generaltypes.StructuredSQLErrorFromError(err)
			document.Statements = append(document.Statements, jsonStatementResult{
				Statement: statement,
				// Reclassify from the SQL text here because failed statements do not
				// return a QueryResult carrying database-populated metadata.
				StatementType: generaltypes.ClassifyStatement(statement),
				RowsAffected:  0,
				Columns:       []string{},
				Rows:          [][]any{},
				Truncated:     false,
				Error:         &structuredErr,
			})

			executionErr = err

			break
		}

		document.Statements = append(document.Statements, jsonStatementResult{
			Statement:     statement,
			StatementType: queryResult.StatementType(),
			RowsAffected:  queryResult.RowsAffected(),
			Columns:       queryResult.ColumnNames(),
			Rows:          queryResult.Values(),
			Truncated:     queryResult.Truncated(),
		})

		if queryResult.Truncated() {
			if err := printTruncationFooter(os.Stderr, len(queryResult.Rows())); err != nil {
				return err
			}
		}
	}

	if err := encodeJSONDocument(output, document, jsonFormat); err != nil {
		return err
	}

	if executionErr != nil {
		return renderedJSONSQLError{cause: executionErr}
	}

	return nil
}

func encodeJSONDocument(output io.Writer, payload any, jsonFormat JSONFormat) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if normalizeJSONFormat(jsonFormat) == JSONFormatPretty {
		encoder.SetIndent("", "  ")
	}

	return encoder.Encode(payload)
}
