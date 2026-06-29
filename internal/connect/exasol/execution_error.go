// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import generaltypes "github.com/exasol/exasol-personal/internal/connect/types"

type executionError struct {
	cause      error
	structured generaltypes.StructuredSQLError
}

func (e executionError) Error() string {
	return e.cause.Error()
}

func (e executionError) Unwrap() error {
	return e.cause
}

func (e executionError) StructuredSQLError() generaltypes.StructuredSQLError {
	return e.structured
}
