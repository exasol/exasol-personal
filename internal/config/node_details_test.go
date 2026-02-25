// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDeploymentHostPort_NoNodes(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{}}
	host, port, err := nodeDetails.GetDeploymentHostPort()
	require.Error(t, err)
	require.Empty(t, host)
	require.Equal(t, 0, port)
}

func TestGetDeploymentHostPort_N11Preferred_DnsName(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			DnsName:  "db.example.local",
			PublicIp: "1.2.3.4",
			Database: nodeDetailsNodesDatabase{DbPort: "8563"},
		},
		"n12": {
			DnsName:  "ignored.local",
			PublicIp: "5.6.7.8",
			Database: nodeDetailsNodesDatabase{DbPort: "8563"},
		},
	}}

	host, port, err := nodeDetails.GetDeploymentHostPort()
	require.NoError(t, err)
	require.Equal(t, "db.example.local", host)
	require.Equal(t, 8563, port)
}

func TestGetDeploymentHostPort_FallbackFirstNode_WhenN11Missing(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n12": {
			DnsName:  "n12.example.local",
			PublicIp: "5.6.7.8",
			Database: nodeDetailsNodesDatabase{DbPort: "8563"},
		},
		"n10": {
			DnsName:  "n10.example.local",
			PublicIp: "9.9.9.9",
			Database: nodeDetailsNodesDatabase{DbPort: "8563"},
		},
	}}

	// First sorted node should be n10
	host, port, err := nodeDetails.GetDeploymentHostPort()
	require.NoError(t, err)
	require.Equal(t, "n10.example.local", host)
	require.Equal(t, 8563, port)
}

func TestGetDeploymentHostPort_UsesPublicIp_WhenDnsEmpty(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			DnsName:  "",
			PublicIp: "1.2.3.4",
			Database: nodeDetailsNodesDatabase{DbPort: "8563"},
		},
	}}

	host, port, err := nodeDetails.GetDeploymentHostPort()
	require.NoError(t, err)
	require.Equal(t, "1.2.3.4", host)
	require.Equal(t, 8563, port)
}

func TestGetDeploymentHostPort_Error_WhenHostEmpty(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			DnsName:  "",
			PublicIp: "",
			Database: nodeDetailsNodesDatabase{DbPort: "8563"},
		},
	}}

	_, _, err := nodeDetails.GetDeploymentHostPort()
	require.Error(t, err)
}

func TestGetDeploymentHostPort_Error_WhenPortEmpty(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			DnsName:  "db.example.local",
			PublicIp: "1.2.3.4",
			Database: nodeDetailsNodesDatabase{DbPort: ""},
		},
	}}

	_, _, err := nodeDetails.GetDeploymentHostPort()
	require.Error(t, err)
}

func TestGetDeploymentHostPort_Error_WhenPortNonNumeric(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			DnsName:  "db.example.local",
			PublicIp: "1.2.3.4",
			Database: nodeDetailsNodesDatabase{DbPort: "eight-five-six-three"},
		},
	}}

	_, _, err := nodeDetails.GetDeploymentHostPort()
	require.Error(t, err)
}

func TestGetDeploymentHostPort_Error_WhenPortOutOfRange(t *testing.T) {
	t.Parallel()

	nodeDetails := &NodeDetails{Nodes: map[string]nodeDetailsNode{
		"n11": {
			DnsName:  "db.example.local",
			PublicIp: "1.2.3.4",
			Database: nodeDetailsNodesDatabase{DbPort: "70000"},
		},
	}}

	_, _, err := nodeDetails.GetDeploymentHostPort()
	require.Error(t, err)
}
