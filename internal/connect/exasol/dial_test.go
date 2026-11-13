// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exasol

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	driver "github.com/exasol/exasol-driver-go"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestHostDialTimeoutReservesTimeForRemainingHosts(t *testing.T) {
	t.Parallel()
	// Given
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	// When
	budget := hostDialTimeout(ctx, 2)
	// Then
	require.Positive(t, budget)
	require.LessOrEqual(t, budget, time.Second/2)
	require.Equal(t, maximumHostDialTimeout, hostDialTimeout(t.Context(), 2))
}

func TestHostDialTimeoutIsCappedAndExpiredDeadlinesHaveNoBudget(t *testing.T) {
	t.Parallel()
	// Given
	long, cancelLong := context.WithTimeout(t.Context(), time.Minute)
	defer cancelLong()
	expired, cancelExpired := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancelExpired()
	// When / Then
	require.Equal(t, maximumHostDialTimeout, hostDialTimeout(long, 2))
	require.Zero(t, hostDialTimeout(expired, 2))
}

func TestMultiHostDialDoesNotRetryLoginRejection(t *testing.T) {
	t.Parallel()
	// Given
	var logins atomic.Int32
	server := loginRejectingServer(t, &logins)
	address, ok := server.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	dsn := driver.NewConfig("sys", "secret").Host("localhost,127.0.0.1").
		Port(address.Port).Encryption(false).String()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	// When
	_, err := defaultConnectFunc(ctx, dsn)
	// Then
	require.ErrorContains(t, err, "login rejected")
	require.EqualValues(t, 1, logins.Load())
}

func loginRejectingServer(t *testing.T, logins *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter, request *http.Request,
	) {
		socket, err := (&websocket.Upgrader{}).Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		var command map[string]any
		if err := socket.ReadJSON(&command); err != nil {
			return
		}
		logins.Add(1)
		if err := socket.WriteJSON(map[string]any{
			"status":    "error",
			"exception": map[string]any{"text": "login rejected", "sqlCode": "08004"},
		}); err != nil {
			return
		}
		if err := socket.ReadJSON(&command); err != nil {
			return
		}
		_ = socket.WriteJSON(map[string]any{"status": "ok"})
	}))
	t.Cleanup(server.Close)

	return server
}
