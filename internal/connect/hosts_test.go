// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDatabaseHosts(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		info config.ConnectionInfo
		want string
		err  string
	}{
		{
			name: "legacy retains its exact saved host",
			info: config.ConnectionInfo{
				Host: "primary.example", DBPort: 8563,
				Endpoints: []config.ConnectionEndpoint{
					{Host: "203.0.113.12", DBPort: 8563, Connectable: true},
				},
			},
			want: "primary.example",
		},
		{
			name: "multi-node uses eligible hosts and removes duplicates",
			info: config.ConnectionInfo{
				Host: "primary.example", DBPort: 8563, MultiNode: true,
				Endpoints: []config.ConnectionEndpoint{
					{Host: "203.0.113.11", DBPort: 8563, Connectable: true},
					{Host: "203.0.113.12", DBPort: 8563, Connectable: true},
					{Host: "203.0.113.12", DBPort: 8563, Connectable: true},
					{Host: "203.0.113.13", DBPort: 8563},
				},
			},
			want: "203.0.113.11,203.0.113.12",
		},
		{
			name: "mismatched ports fail clearly",
			info: config.ConnectionInfo{
				DBPort: 8563, MultiNode: true,
				Endpoints: []config.ConnectionEndpoint{
					{Node: "n12", Host: "203.0.113.12", DBPort: 9999, Connectable: true},
				},
			},
			err: "shared database port: node n12 uses 9999 instead of 8563",
		},
		{
			name: "missing eligible hosts fail clearly",
			info: config.ConnectionInfo{DBPort: 8563, MultiNode: true},
			err:  "no connectable public database endpoints",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Given
			info := test.info
			// When
			hosts, err := databaseHosts(&info)
			// Then
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, hosts)
		})
	}
}
