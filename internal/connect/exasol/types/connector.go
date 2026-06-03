// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"context"
	"database/sql/driver"
)

//go:generate go tool counterfeiter -generate

type ConnectFunc func(input string) (ExasolConnector, error)

//counterfeiter:generate . ExasolConnector
type ExasolConnector interface {
	Exec(query string, args []driver.Value) (driver.Result, error)
	QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error)
	Close() error
}
