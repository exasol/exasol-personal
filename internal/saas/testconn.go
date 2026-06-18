// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package saas

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
	generaltypes "github.com/exasol/exasol-personal/internal/connect/types"
)

// Check is one step of the connection test. A check with Warn set reports a
// non-fatal issue (advisory) that does not fail the overall test.
type Check struct {
	Name   string
	OK     bool
	Warn   bool
	Detail string
}

// ConnTestInput bundles the inputs for a connection test.
type ConnTestInput struct {
	DBUUID   string
	Username string // SaaS database user
	Token    string // used as the database connection credential
	EgressIP string // optional; when set, verified against the allowlist
}

// RunConnectionTest performs a non-destructive, fail-fast connectivity check
// against a SaaS target. It returns the checks performed (in order), whether all
// passed, and the resolved target on success.
//
// No data is transferred and neither database is mutated.
func RunConnectionTest(
	ctx context.Context,
	api API,
	newDB DBFactory,
	accountID, region string,
	input ConnTestInput,
) ([]Check, *config.DeploymentSaaS, bool) {
	slog.Info("running SaaS connection test", "db", input.DBUUID, "account", accountID)

	var checks []Check
	add := func(name string, ok bool, detail string) bool {
		checks = append(checks, Check{Name: name, OK: ok, Detail: detail})
		slog.Debug("connection check", "name", name, "ok", ok, "detail", detail)

		return ok
	}
	// addWarn records a non-fatal, advisory check that never aborts the test.
	addWarn := func(name string, ok bool, detail string) {
		checks = append(checks, Check{Name: name, OK: ok, Warn: !ok, Detail: detail})
		slog.Debug("connection check (advisory)", "name", name, "ok", ok, "detail", detail)
	}

	account, err := api.ValidateToken(ctx)
	if !add("token valid", err == nil, accountDetail(account, err)) {
		return checks, nil, false
	}

	database, err := api.ResolveDatabase(ctx, input.DBUUID)
	if !add("database resolved", err == nil, databaseDetail(database, err)) {
		return checks, nil, false
	}
	if !add("database running", isRunning(database.Status), "status="+database.Status) {
		return checks, nil, false
	}

	// The allowlist check is advisory: if the token cannot read the allowlist, or
	// the IP is absent, we warn but still attempt the connection — the reachability
	// check below is the real verdict.
	if input.EgressIP != "" {
		entries, listErr := api.ListAllowedIPs(ctx)
		switch {
		case listErr != nil:
			addWarn("egress ip allowed", false, "could not verify: "+listErr.Error())
		case containsIP(entries, input.EgressIP):
			addWarn("egress ip allowed", true, input.EgressIP)
		default:
			addWarn("egress ip allowed", false, input.EgressIP+" not in allowlist")
		}
	}

	resolved := FromDatabase(accountID, region, input.Username, database)

	// From here the target endpoint is known; return it even on failure so the
	// caller can surface the connection string for verification.
	conn, err := connectTarget(ctx, newDB, &resolved, input.Token)
	if !add("endpoint reachable + authenticated", err == nil, reachDetail(&resolved, err)) {
		return checks, &resolved, false
	}
	defer conn.Close()

	_, execErr := conn.Exec(ctx, "SELECT 1", 1)
	if !add("SELECT 1 succeeded", execErr == nil, errDetail(execErr)) {
		return checks, &resolved, false
	}

	// The token determines the session user; if a db user was supplied, verify it
	// matches CURRENT_USER (advisory — a mismatch does not fail the connection).
	currentUser := queryCurrentUser(ctx, conn)
	if input.Username != "" &&
		!strings.EqualFold(strings.TrimSpace(currentUser), strings.TrimSpace(input.Username)) {
		addWarn("connected user", false, "expected "+input.Username+", got "+currentUser)
	} else {
		addWarn("connected user", true, currentUser)
	}

	slog.Info("SaaS connection test passed",
		"host", resolved.Host, "port", resolved.Port, "user", currentUser)

	return checks, &resolved, true
}

// queryCurrentUser returns the session user, or "" if it cannot be determined.
func queryCurrentUser(ctx context.Context, conn generaltypes.Databaser) string {
	result, err := conn.Exec(ctx, "SELECT CURRENT_USER", 1)
	if err != nil {
		return ""
	}
	rows := result.Rows()
	if len(rows) > 0 && len(rows[0]) > 0 {
		return rows[0][0]
	}

	return ""
}

// statusRunning is the SaaS database status that permits connecting.
const statusRunning = "running"

func isRunning(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case statusRunning, "ok", "available", "ready":
		return true
	default:
		return false
	}
}

func containsIP(entries []AllowedIP, ip string) bool {
	cidr, err := ToCIDR(ip)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.CidrIp), cidr) {
			return true
		}
	}

	return false
}

func accountDetail(account *Account, err error) string {
	if err != nil {
		return errDetail(err)
	}

	return "account " + account.ID
}

func databaseDetail(db *Database, err error) string {
	if err != nil {
		return errDetail(err)
	}

	return db.ID
}

func reachDetail(target *config.DeploymentSaaS, err error) string {
	if err != nil {
		return errDetail(err)
	}

	return fmt.Sprintf("%s:%d", target.Host, target.Port)
}

func errDetail(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
