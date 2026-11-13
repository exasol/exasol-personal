// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"reflect"
	"testing"
)

func TestConnectionEndpointsIncludePublicNodesInStableOrder(t *testing.T) {
	t.Parallel()
	for _, multiNode := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "unlimited"}[multiNode], func(t *testing.T) {
			t.Parallel()
			// Given
			info := endpointTestDeployment()
			info.Nodes["n13"] = DeploymentNode{
				PrivateIp: "10.0.0.13",
				Database:  DeploymentDatabase{DbPort: "8563"},
			}
			info.Nodes["n14"] = DeploymentNode{
				PublicIp: "203.0.113.14",
				Database: DeploymentDatabase{DbPort: "65536"},
			}
			// When
			endpoints := info.connectionEndpoints(multiNode)
			// Then
			want := []ConnectionEndpoint{
				{Node: "n11", Host: "203.0.113.11", DBPort: 8563, Eligible: true},
				{Node: "n12", Host: "203.0.113.12", DBPort: 8563, Eligible: multiNode},
			}
			if !reflect.DeepEqual(endpoints, want) {
				t.Fatalf("endpoints = %#v, want %#v", endpoints, want)
			}
		})
	}
}

func TestConnectionEndpointsPreservePerNodePorts(t *testing.T) {
	t.Parallel()
	// Given
	info := endpointTestDeployment()
	node := info.Nodes["n12"]
	node.Database.DbPort = "9000"
	info.Nodes["n12"] = node
	// When
	endpoints := info.connectionEndpoints(true)
	// Then
	if endpoints[1].DBPort != 9000 || !endpoints[1].Eligible {
		t.Fatalf("endpoint must report its actual port and license eligibility: %#v", endpoints[1])
	}
}

func TestConnectionEndpointsFallBackToSingularConnection(t *testing.T) {
	t.Parallel()
	// Given
	info := endpointTestDeployment()
	info.Nodes = nil
	// When
	endpoints := info.connectionEndpoints(false)
	// Then
	want := []ConnectionEndpoint{{Host: "primary.example", DBPort: 8563, Eligible: true}}
	if !reflect.DeepEqual(endpoints, want) {
		t.Fatalf("endpoints = %#v, want %#v", endpoints, want)
	}
}

func TestConnectionEndpointsPreserveLegacyHostWithoutPrimaryPublicAddress(t *testing.T) {
	t.Parallel()
	// Given
	info := endpointTestDeployment()
	info.Nodes["n11"] = DeploymentNode{}
	// When
	endpoints := info.connectionEndpoints(false)
	// Then
	want := []ConnectionEndpoint{
		{Host: "primary.example", DBPort: 8563, Eligible: true},
		{Node: "n12", Host: "203.0.113.12", DBPort: 8563, Eligible: false},
	}
	if !reflect.DeepEqual(endpoints, want) {
		t.Fatalf("endpoints = %#v, want %#v", endpoints, want)
	}
}

func TestLocalConnectionEndpointsUseOnlyTheLocalConnection(t *testing.T) {
	t.Parallel()
	// Given
	info := endpointTestDeployment()
	info.Backend = "local"
	// When
	endpoints := info.connectionEndpoints(true)
	// Then
	want := []ConnectionEndpoint{{Host: "primary.example", DBPort: 8563, Eligible: true}}
	if !reflect.DeepEqual(endpoints, want) {
		t.Fatalf("endpoints = %#v, want %#v", endpoints, want)
	}
}

func TestLegacyConnectionEndpointsFollowTheSavedHost(t *testing.T) {
	t.Parallel()
	// Given
	info := endpointTestDeployment()
	info.Connection.Host = "203.0.113.12"
	// When
	endpoints := info.connectionEndpoints(false)
	// Then
	if endpoints[0].Eligible || !endpoints[1].Eligible {
		t.Fatalf("eligibility must follow the saved host: %#v", endpoints)
	}
}

func TestLegacyConnectionEndpointsPreserveCustomSavedHost(t *testing.T) {
	t.Parallel()
	// Given
	info := endpointTestDeployment()
	info.Connection.Host = "custom.example"
	// When
	endpoints := info.connectionEndpoints(false)
	// Then
	if len(endpoints) != 3 || endpoints[0].Host != "custom.example" ||
		!endpoints[0].Eligible || endpoints[1].Eligible || endpoints[2].Eligible {
		t.Fatalf("eligibility must preserve the custom connection: %#v", endpoints)
	}
}

func TestConnectionInfoReadsSavedEligibility(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, backend, manifest string
		multiNode, wantError    bool
	}{
		{name: "missing manifest"},
		{name: "legacy manifest", manifest: "name: legacy\n"},
		{
			name:      "unlimited",
			manifest:  "databaseConnection:\n  allowAllNodes: true\n",
			multiNode: true,
		},
		{name: "local", backend: "local", manifest: "databaseConnection:\n  allowAllNodes: true\n"},
		{name: "invalid manifest", manifest: "databaseConnection: [", wantError: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Given
			deployment := NewDeploymentDir(t.TempDir())
			if err := WriteSecrets(deployment.Root(), &Secrets{DbPassword: "secret"}); err != nil {
				t.Fatal(err)
			}
			if test.manifest != "" {
				if err := os.MkdirAll(deployment.InstallationDir(), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					deployment.InstallManifestPath(),
					[]byte(test.manifest),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			}
			info := endpointTestDeployment()
			info.Backend = test.backend
			// When
			connection, err := info.ConnectionInfo(deployment)
			// Then
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, want error %v", err, test.wantError)
			}
			if err == nil &&
				(connection.MultiNode != test.multiNode || connection.Host != "primary.example") {
				t.Fatalf("unexpected eligibility or changed singular host: %#v", connection)
			}
		})
	}
}

func endpointTestDeployment() *DeploymentInfo {
	return &DeploymentInfo{
		Connection: &DeploymentConnection{Host: "primary.example", DBPort: 8563},
		Nodes: map[string]DeploymentNode{
			"n12": {PublicIp: "203.0.113.12", Database: DeploymentDatabase{DbPort: "8563"}},
			"n11": {
				PublicIp: "203.0.113.11",
				DnsName:  "primary.example",
				Database: DeploymentDatabase{DbPort: "8563"},
			},
		},
	}
}
