// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/exasol/exasol-driver-go/pkg/connection"
	"github.com/exasol/exasol-driver-go/pkg/dsn"
	"github.com/exasol/exasol-personal/internal/connect/exasol/types"
)

const maximumHostDialTimeout = 10 * time.Second

type contextConnectFunc func(context.Context, string) (types.ExasolConnector, error)

func defaultConnectFunc(ctx context.Context, input string) (types.ExasolConnector, error) {
	parsed, err := dsn.ParseDSN(input)
	if err != nil {
		return nil, err
	}
	hosts := strings.Split(parsed.Host, ",")
	//nolint:gosec // G404: host order balances sessions; randomness is not a security boundary.
	rand.Shuffle(len(hosts), func(left, right int) {
		hosts[left], hosts[right] = hosts[right], hosts[left]
	})
	for index, host := range hosts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		parsed.Host = host
		database := &connection.Connection{
			Config: dsn.ToInternalConfig(parsed), Ctx: ctx, IsClosed: true,
		}
		err = connectTransport(ctx, database, len(hosts)-index, len(hosts) > 1)
		if err != nil {
			continue
		}
		if err := database.Login(ctx); err != nil {
			// Driver Close waits for a disconnect reply with no context-aware close API.
			go func() { _ = database.Close() }()

			return nil, err
		}

		return database, nil
	}

	return nil, err
}

func connectTransport(
	ctx context.Context, database *connection.Connection, hostsLeft int, multipleHosts bool,
) error {
	if !multipleHosts {
		return database.Connect()
	}
	dialCtx, cancel := context.WithTimeout(ctx, hostDialTimeout(ctx, hostsLeft))
	database.Ctx = dialCtx
	err := database.Connect()
	cancel()
	// The dial context belongs only to the handshake; later operations use the caller's context.
	database.Ctx = ctx

	return err
}

func hostDialTimeout(ctx context.Context, hostsLeft int) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return min(maximumHostDialTimeout, max(0, time.Until(deadline))/time.Duration(hostsLeft))
	}

	return maximumHostDialTimeout
}
