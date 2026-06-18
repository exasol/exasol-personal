// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"net/http"
	"strings"

	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

// fakeResult is a canned query result.
type fakeResult struct {
	cols []string
	rows [][]string
}

func (f fakeResult) ColumnNames() []string { return f.cols }
func (f fakeResult) Rows() [][]string      { return f.rows }
func (fakeResult) Truncated() bool         { return false }

// fakeDB is a Databaser whose Exec answers are matched by query substring.
type fakeDB struct {
	responses  map[string]fakeResult
	executed   []string
	connectErr error
	execErr    error
}

func (d *fakeDB) Connect(_ context.Context) error { return d.connectErr }
func (*fakeDB) Close() error                      { return nil }

func (d *fakeDB) Exec(_ context.Context, query string, _ int) (generaltypes.QueryResulter, error) {
	d.executed = append(d.executed, query)
	if d.execErr != nil {
		return nil, d.execErr
	}
	for substr, res := range d.responses {
		if strings.Contains(query, substr) {
			return res, nil
		}
	}

	return fakeResult{}, nil
}

// fakeAPI is a canned SaaS API implementation.
type fakeAPI struct {
	account    *Account
	accountErr error
	db         *Database
	dbErr      error
	ips        []AllowedIP
	ipsErr     error
	added      []AllowedIP
	egressIP   string
	egressErr  error
}

func (a *fakeAPI) ValidateToken(_ context.Context) (*Account, error) {
	return a.account, a.accountErr
}

func (a *fakeAPI) ResolveDatabase(_ context.Context, _ string) (*Database, error) {
	return a.db, a.dbErr
}

func (a *fakeAPI) ListAllowedIPs(_ context.Context) ([]AllowedIP, error) {
	return a.ips, a.ipsErr
}

func (a *fakeAPI) AddAllowedIP(_ context.Context, entry AllowedIP) error {
	a.added = append(a.added, entry)
	return nil
}

func (a *fakeAPI) DetectEgressIP(_ context.Context) (string, error) {
	return a.egressIP, a.egressErr
}

// fakeDoer is an injectable HTTP transport.
type fakeDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (d fakeDoer) Do(req *http.Request) (*http.Response, error) { return d.fn(req) }
