// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package types

import (
	"context"
	"database/sql/driver"

	"github.com/exasol/exasol-driver-go/pkg/types"
)

//go:generate go tool counterfeiter -generate

type ConnectFunc func(input string) (ExasolConnector, error)

//counterfeiter:generate . ExasolConnector
type ExasolConnector interface {
	Exec(query string, args []driver.Value) (driver.Result, error)
	SimpleExec(ctx context.Context, query string) (*types.SqlQueriesResponse, error)
	Close() error
}
