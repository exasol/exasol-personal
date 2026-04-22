// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"net"
)

const loopbackHost = "127.0.0.1"

func AllocatePort(excluded map[int]struct{}) (int, error) {
	const maxAttempts = 32

	for range maxAttempts {
		listener, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, "0"))
		if err != nil {
			return 0, fmt.Errorf("failed to allocate local port: %w", err)
		}

		tcpAddr, ok := listener.Addr().(*net.TCPAddr)
		closeErr := listener.Close()
		if closeErr != nil {
			return 0, fmt.Errorf("failed to release local port probe: %w", closeErr)
		}
		if !ok {
			return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
		}
		if _, exists := excluded[tcpAddr.Port]; exists {
			continue
		}

		return tcpAddr.Port, nil
	}

	return 0, fmt.Errorf("failed to find an unused local port after %d attempts", maxAttempts)
}
