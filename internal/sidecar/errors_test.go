// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar_test

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

func TestStartupErrorNamesTheReportedCause(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, diagnostic, expected string }{
		{
			"pull progress precedes the cause",
			"Trying to pull docker.io/exasol/mcp-server:9.9.9...\n" +
				"Error: reading manifest 9.9.9: requested access to the resource is denied",
			"requested access to the resource is denied",
		},
		{
			"single error line",
			"Error: rootlessport listen tcp 127.0.0.1:4896: bind: address already in use",
			"address already in use",
		},
		{
			"docker run error precedes its help hint",
			"docker: Error response from daemon: driver failed programming external " +
				"connectivity: Bind for 127.0.0.1:8563 failed: port is already allocated\n\n" +
				"Run 'docker run --help' for more information",
			"port is already allocated",
		},
		{
			"docker daemon error without the CLI prefix",
			"Error response from daemon: network with name exasol-services already exists",
			"already exists",
		},
		{
			"no error prefix falls back to the last line",
			"first line\nlast line",
			"last line",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// When
			err := sidecar.NewStartupError("example", fixture.diagnostic, "exit status 125")
			// Then
			if !strings.Contains(err.Cause, fixture.expected) {
				t.Fatalf("cause %q omits %q", err.Cause, fixture.expected)
			}
		})
	}
}
