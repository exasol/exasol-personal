// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"sort"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func writeTestNodeDetails(t *testing.T, deployment config.DeploymentDir, nodeNames ...string) {
	t.Helper()

	nodes := make(map[string]config.DeploymentNode, len(nodeNames))
	for _, name := range nodeNames {
		keyRelPath := "keys/" + name + ".pem"
		writeTestSSHKeyFile(t, deployment, keyRelPath)
		nodes[name] = config.DeploymentNode{
			PublicIp: "203.0.113.10",
			Ssh: config.DeploymentSSH{
				Username: "sys",
				Port:     "22",
				KeyFile:  keyRelPath,
			},
		}
	}

	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Nodes:        nodes,
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}
}

func TestNodeLookupFind_MatchesNodesByGlob(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeTestNodeDetails(t, deployment, "n11", "n12", "n21")

	lookup := NewNodeLookup(deployment)

	nodes, err := lookup.Find("n1*")
	if err != nil {
		t.Fatalf("expected glob match to succeed, got %v", err)
	}

	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		names = append(names, node.Name)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "n11" || names[1] != "n12" {
		t.Fatalf("expected [n11 n12], got %v", names)
	}
}

func TestNodeLookupFind_MatchesAllNodesWithWildcard(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeTestNodeDetails(t, deployment, "n11", "n21")

	lookup := NewNodeLookup(deployment)

	nodes, err := lookup.Find("*")
	if err != nil {
		t.Fatalf("expected wildcard match to succeed, got %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected both nodes to match, got %+v", nodes)
	}
}

func TestNodeLookupFind_NoMatchReturnsEmptyList(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeTestNodeDetails(t, deployment, "n11")

	lookup := NewNodeLookup(deployment)

	nodes, err := lookup.Find("does-not-exist")
	if err != nil {
		t.Fatalf("expected no error for a non-matching glob, got %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected no nodes to match, got %+v", nodes)
	}
}

func TestNodeLookupFind_InvalidGlobReturnsError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	writeTestNodeDetails(t, deployment, "n11")

	lookup := NewNodeLookup(deployment)

	_, err := lookup.Find("[")
	if err == nil {
		t.Fatal("expected an error for an invalid glob pattern")
	}
}

func TestNodeLookupFind_MissingNodeDetailsReturnsError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())

	lookup := NewNodeLookup(deployment)

	_, err := lookup.Find("*")
	if err == nil {
		t.Fatal("expected an error when node details are missing")
	}
}

func TestNodeLookupFind_MissingKeyFileReturnsError(t *testing.T) {
	t.Parallel()

	deployment := config.NewDeploymentDir(t.TempDir())
	if err := config.WriteDeploymentInfo(deployment.Root(), &config.DeploymentInfo{
		Backend:      "tofu",
		DeploymentId: "dep-1",
		Nodes: map[string]config.DeploymentNode{
			"n11": {
				PublicIp: "203.0.113.10",
				Ssh: config.DeploymentSSH{
					Username: "sys",
					Port:     "22",
					KeyFile:  "keys/missing.pem",
				},
			},
		},
	}); err != nil {
		t.Fatalf("failed to write deployment info: %v", err)
	}

	lookup := NewNodeLookup(deployment)

	_, err := lookup.Find("*")
	if err == nil {
		t.Fatal("expected an error for a missing SSH key file")
	}
}
