// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import "context"

//go:generate go tool counterfeiter -generate

// QueryResulter describes a query result.
type QueryResulter interface {
	ColumnNames() []string
	Rows() [][]string
}

// Databaser describes an interface for interacting with a running database instance.
// It provides methods for establishing SQL connections and executing queries.
type Databaser interface {
	Connect(ctx context.Context) error
	Exec(ctx context.Context, query string) (QueryResulter, error)
	Close() error
}

// Sheller describes a way to interact with an interactive
// shell processor.
//
//counterfeiter:generate . LineReader
type LineReader interface {
	Readline() (string, error)
	Close() error
}

// TableFormatter describes a way to format an ASCII table.
type TableFormatter interface {
	SetHeader(header []string)
	SetRows(rows [][]string) error
	Render() error
}
