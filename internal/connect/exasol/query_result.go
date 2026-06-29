// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import generaltypes "github.com/exasol/exasol-personal/internal/connect/types"

// QueryResult holds the materialized result of a query.
type QueryResult struct {
	columnNames   []string
	rows          [][]string
	values        [][]any
	statementType generaltypes.StatementType
	rowsAffected  int64
	truncated     bool
}

func (qr *QueryResult) ColumnNames() []string {
	return qr.columnNames
}

func (qr *QueryResult) Rows() [][]string {
	return qr.rows
}

func (qr *QueryResult) Values() [][]any {
	return qr.values
}

func (qr *QueryResult) StatementType() generaltypes.StatementType {
	return qr.statementType
}

func (qr *QueryResult) RowsAffected() int64 {
	return qr.rowsAffected
}

func (qr *QueryResult) Truncated() bool {
	return qr.truncated
}
