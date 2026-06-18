// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"testing"

	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
	"github.com/stretchr/testify/require"
)

const (
	testDBUUID    = "db-1"
	testAccount   = "ORG-1"
	testSelectOne = "SELECT 1"
	testEgressIP  = "203.0.113.7"
	testDBUser    = "dbuser"
)

func okFactory(db *fakeDB) DBFactory {
	return func(_, _ string, _ int) (generaltypes.Databaser, error) {
		return db, nil
	}
}

func TestRunConnectionTest_AllPass(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		account: &Account{ID: testAccount},
		db: &Database{
			ID:         testDBUUID,
			Status:     statusRunning,
			Connection: DatabaseConnection{Host: "h", Port: 8563},
		},
		ips: []AllowedIP{{CidrIp: "203.0.113.7/32"}},
	}
	target := &fakeDB{responses: map[string]fakeResult{
		testSelectOne:         {rows: [][]string{{"1"}}},
		"SELECT CURRENT_USER": {rows: [][]string{{testDBUser}}},
	}}

	checks, resolved, allOK := RunConnectionTest(
		context.Background(),
		api,
		okFactory(target),
		testAccount,
		"eu",
		ConnTestInput{
			DBUUID:   testDBUUID,
			Username: testDBUser,
			Token:    "tok",
			EgressIP: testEgressIP,
		},
	)

	require.True(t, allOK)
	require.NotNil(t, resolved)
	require.Equal(t, "h", resolved.Host)
	require.Len(t, checks, 7)
	for _, c := range checks {
		require.True(t, c.OK, "check %q should pass: %s", c.Name, c.Detail)
	}
}

func TestRunConnectionTest_UserMismatchIsAdvisory(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		account: &Account{ID: testAccount},
		db:      &Database{ID: testDBUUID, Status: statusRunning},
	}
	target := &fakeDB{responses: map[string]fakeResult{
		testSelectOne:         {rows: [][]string{{"1"}}},
		"SELECT CURRENT_USER": {rows: [][]string{{"someoneelse"}}},
	}}

	checks, _, allOK := RunConnectionTest(context.Background(), api, okFactory(target),
		testAccount, "eu", ConnTestInput{DBUUID: testDBUUID, Username: testDBUser, Token: "tok"})

	require.True(t, allOK)
	user := findCheck(t, checks, "connected user")
	require.True(t, user.Warn)
	require.Contains(t, user.Detail, "someoneelse")
}

func TestRunConnectionTest_StopsAtFirstFailure(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{accountErr: ErrUnauthorized}

	checks, resolved, allOK := RunConnectionTest(context.Background(), api, okFactory(&fakeDB{}),
		testAccount, "eu", ConnTestInput{DBUUID: testDBUUID})

	require.False(t, allOK)
	require.Nil(t, resolved)
	require.Len(t, checks, 1)
	require.False(t, checks[0].OK)
}

func TestRunConnectionTest_EgressNotAllowedIsAdvisory(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		account: &Account{ID: testAccount},
		db:      &Database{ID: testDBUUID, Status: statusRunning},
		ips:     []AllowedIP{{CidrIp: "198.51.100.1/32"}},
	}
	target := &fakeDB{responses: map[string]fakeResult{testSelectOne: {rows: [][]string{{"1"}}}}}

	checks, resolved, allOK := RunConnectionTest(context.Background(), api, okFactory(target),
		testAccount, "eu", ConnTestInput{DBUUID: testDBUUID, EgressIP: testEgressIP})

	// The connection succeeds, so an unverified/absent allowlist entry is only a warning.
	require.True(t, allOK)
	require.NotNil(t, resolved)
	egress := findCheck(t, checks, "egress ip allowed")
	require.False(t, egress.OK)
	require.True(t, egress.Warn)
}

func TestRunConnectionTest_AllowlistForbiddenIsAdvisory(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		account: &Account{ID: testAccount},
		db:      &Database{ID: testDBUUID, Status: statusRunning},
		ipsErr:  ErrForbidden,
	}
	target := &fakeDB{responses: map[string]fakeResult{testSelectOne: {rows: [][]string{{"1"}}}}}

	checks, _, allOK := RunConnectionTest(context.Background(), api, okFactory(target),
		testAccount, "eu", ConnTestInput{DBUUID: testDBUUID, EgressIP: testEgressIP})

	require.True(t, allOK)
	egress := findCheck(t, checks, "egress ip allowed")
	require.True(t, egress.Warn)
	require.Contains(t, egress.Detail, "could not verify")
}

func findCheck(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found", name)

	return Check{}
}
