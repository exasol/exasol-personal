// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package connect

import (
	"os"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDatabaseAddress(t *testing.T) {
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
					{Host: "203.0.113.12", DBPort: 8563, Eligible: true},
				},
			},
			want: "primary.example",
		},
		{
			name: "multi-node uses eligible hosts and removes duplicates",
			info: config.ConnectionInfo{
				Host: "primary.example", DBPort: 8563, AllowAllNodes: true,
				Endpoints: []config.ConnectionEndpoint{
					{Host: "203.0.113.11", DBPort: 8563, Eligible: true},
					{Host: "203.0.113.12", DBPort: 8563, Eligible: true},
					{Host: "203.0.113.12", DBPort: 8563, Eligible: true},
					{Host: "203.0.113.13", DBPort: 8563},
				},
			},
			want: "203.0.113.11,203.0.113.12",
		},
		{
			name: "mismatched ports fail clearly",
			info: config.ConnectionInfo{
				DBPort: 8563, AllowAllNodes: true,
				Endpoints: []config.ConnectionEndpoint{
					{Node: "n11", Host: "203.0.113.11", DBPort: 8563, Eligible: true},
					{Node: "n12", Host: "203.0.113.12", DBPort: 9999, Eligible: true},
				},
			},
			err: "shared database port: node n12 uses 9999 instead of 8563",
		},
		{
			name: "missing eligible hosts fail clearly",
			info: config.ConnectionInfo{DBPort: 8563, AllowAllNodes: true},
			err:  "no eligible public database endpoints",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Given
			info := test.info
			// When
			hosts, port, err := databaseAddress(&info)
			// Then
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, hosts)
			require.Equal(t, info.DBPort, port)
		})
	}
}

func TestDatabaseAddressCombinesSavedPolicyAndInfrastructureOutputs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name               string
		primary, secondary bool
		want               string
	}{
		{name: "both exposed", primary: true, secondary: true, want: "203.0.113.11,203.0.113.12"},
		{name: "only primary exposed", primary: true, want: "203.0.113.11"},
		{name: "only secondary exposed", secondary: true, want: "203.0.113.12"},
		{name: "no nodes exposed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Given
			deployment := config.NewDeploymentDir(t.TempDir())
			require.NoError(t, os.MkdirAll(deployment.InstallationDir(), 0o700))
			require.NoError(t, os.WriteFile(deployment.InstallManifestPath(),
				[]byte("databaseConnection:\n  allowAllNodes: true\n"), 0o600))
			require.NoError(
				t,
				config.WriteSecrets(deployment.Root(), &config.Secrets{DbPassword: "secret"}),
			)
			nodes := map[string]config.DeploymentNode{
				"n11": {
					PrivateIp: "10.0.0.11",
					Database:  config.DeploymentDatabase{DbPort: "9000"},
				},
				"n12": {
					PrivateIp: "10.0.0.12",
					Database:  config.DeploymentDatabase{DbPort: "9000"},
				},
			}
			for name, exposed := range map[string]bool{"n11": test.primary, "n12": test.secondary} {
				if exposed {
					node := nodes[name]
					node.PublicIp = "203.0.113." + name[1:]
					nodes[name] = node
				}
			}
			require.NoError(t, config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
				Backend: "tofu", Connection: &config.DeploymentConnection{}, Nodes: nodes,
			}))

			// When
			info, err := config.ResolveConnectionInfo(deployment)
			require.NoError(t, err)
			hosts, port, err := databaseAddress(info)

			// Then
			require.True(t, info.AllowAllNodes)
			if test.want == "" {
				require.ErrorContains(t, err, "no eligible public database endpoints")
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, hosts)
			require.Equal(t, 9000, port)
		})
	}
}
