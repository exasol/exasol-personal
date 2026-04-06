// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadNodeDetails_PreservesPresetRenderedSSHCommand(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	err := os.WriteFile(filepath.Join(deploymentDir, nodeDetailsFileName), []byte(`{
		"deploymentId": "dep-1",
		"region": "test-region",
		"availabilityZone": "test-az",
		"clusterSize": 1,
		"clusterState": "running",
		"instanceType": "test-type",
		"vpcId": "vpc-1",
		"subnetId": "subnet-1",
		"nodes": {
			"n11": {
				"publicIp": "1.2.3.4",
				"privateIp": "10.0.0.1",
				"dnsName": "db.example.local",
				"instanceId": "i-1",
				"availabilityZone": "test-az",
				"ssh": {
					"username": "ubuntu",
					"keyName": "node-access",
					"keyFile": "node_access.pem",
					"port": "22",
					"command": "ssh -i node_access.pem ubuntu@db.example.local -p 22"
				},
				"tlsCert": "ignored",
				"database": {
					"dbPort": "8563",
					"uiPort": "8443",
					"url": "https://db.example.local:8443"
				}
			}
		}
	}`), 0o600)
	require.NoError(t, err)

	nodeDetails, err := ReadNodeDetails(deploymentDir)
	require.NoError(t, err)

	node := nodeDetails.Nodes["n11"]
	require.Equal(t, NodeAccessKeyFileName, node.Ssh.KeyFile)
	require.Equal(t, "ssh -i node_access.pem ubuntu@db.example.local -p 22", node.Ssh.Command)
}

func TestGetSSHDetails_AllowsAbsoluteLegacyKeyPath(t *testing.T) {
	t.Parallel()

	deploymentDir := t.TempDir()
	absoluteKeyPath := filepath.Join(t.TempDir(), NodeAccessKeyFileName)
	err := os.WriteFile(filepath.Join(deploymentDir, nodeDetailsFileName), []byte(fmt.Sprintf(`{
		"deploymentId": "dep-1",
		"region": "test-region",
		"availabilityZone": "test-az",
		"clusterSize": 1,
		"clusterState": "running",
		"instanceType": "test-type",
		"vpcId": "vpc-1",
		"subnetId": "subnet-1",
		"nodes": {
			"n11": {
				"publicIp": "1.2.3.4",
				"privateIp": "10.0.0.1",
				"dnsName": "db.example.local",
				"instanceId": "i-1",
				"availabilityZone": "test-az",
				"ssh": {
					"username": "ubuntu",
					"keyName": "node-access",
					"keyFile": %q,
					"port": "22",
					"command": %q
				},
				"tlsCert": "ignored",
				"database": {
					"dbPort": "8563",
					"uiPort": "8443",
					"url": "https://db.example.local:8443"
				}
			}
		}
	}`, absoluteKeyPath, "ssh -i "+absoluteKeyPath+" ubuntu@db.example.local -p 22")), 0o600)
	require.NoError(t, err)

	nodeDetails, err := ReadNodeDetails(deploymentDir)
	require.NoError(t, err)

	sshDetails, err := nodeDetails.GetSSHDetails("n11")
	require.NoError(t, err)
	require.Equal(t, absoluteKeyPath, ResolveDeploymentPath(
		sshDetails.KeyFile,
		"/different/deployment/dir"))
}

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
