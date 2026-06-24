// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

// QueryResult holds the materialized result of a query.
type QueryResult struct {
	columnNames []string
	rows        [][]string
	values      [][]any
	truncated   bool
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

func (qr *QueryResult) Truncated() bool {
	return qr.truncated
}
