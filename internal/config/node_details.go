// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

const (
	nodeDetailsFileName   = "deployment.json"
	ConnectionInstruction = "connection-instructions.txt"
)

type NodeDetails struct {
	DeploymentId     string                     `json:"deploymentId"`
	DeploymentState  string                     `json:"deploymentState,omitempty"`
	Region           string                     `json:"region"`
	AvailabilityZone string                     `json:"availabilityZone"`
	ClusterSize      int                        `json:"clusterSize"`
	ClusterState     string                     `json:"clusterState"`
	InstanceType     string                     `json:"instanceType"`
	VpcId            string                     `json:"vpcId"`
	SubnetId         string                     `json:"subnetId"`
	Nodes            map[string]nodeDetailsNode `json:"nodes"`
}

type nodeDetailsNode struct {
	AvailabilityZone string                   `json:"availabilityZone"`
	Database         nodeDetailsNodesDatabase `json:"database"`
	DnsName          string                   `json:"dnsName"`
	InstanceId       string                   `json:"instanceId"`
	PrivateIp        string                   `json:"privateIp"`
	PublicIp         string                   `json:"publicIp"`
	Ssh              nodeDetailsNodesSsh      `json:"ssh"`
	TlsCert          string                   `json:"tlsCert"`
}

type nodeDetailsNodesDatabase struct {
	DbPort string `json:"dbPort"`
	UiPort string `json:"uiPort"`
	Url    string `json:"url"`
}

type nodeDetailsNodesSsh struct {
	Command  string `json:"command"`
	KeyFile  string `json:"keyFile"`
	KeyName  string `json:"keyName"`
	Port     string `json:"port"`
	Username string `json:"username"`
}

type SSHDetails struct {
	Host    string
	Port    string
	User    string
	KeyFile string
}

func ReadNodeDetails(deploymentDir string) (*NodeDetails, error) {
	filepath, exists, err := findExistingFile(deploymentDir, nodeDetailsFileName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf(
			"node details file not found in deployment directory: expected %q in %s",
			nodeDetailsFileName,
			deploymentDir,
		)
	}

	slog.Debug("reading node details file", "file", filepath)

	return readConfig[NodeDetails](filepath, "node details")
}

var ErrNoNodeDetailsFile = errors.New("node details file not found in deployment directory")

// Returns the list of node names, sorted increasingly.
func (s *NodeDetails) ListNodes() []string {
	result := make([]string, 0, len(s.Nodes))
	for nodeName := range s.Nodes {
		result = append(result, nodeName)
	}

	sort.Strings(result)

	return result
}

var ErrUnknownNodeName = errors.New("unknown node name")

func (s *NodeDetails) GetSSHDetails(nodeName string) (*SSHDetails, error) {
	entry, exists := s.Nodes[nodeName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNodeName, nodeName)
	}

	return &SSHDetails{
		Host:    entry.PublicIp,
		User:    entry.Ssh.Username,
		KeyFile: entry.Ssh.KeyFile,
		Port:    entry.Ssh.Port,
	}, nil
}

// GetDeploymentHostPort returns the host-port string
// of the form "{ip}:{port}".
//
// Always returns the host-port for the first node only.
func (s *NodeDetails) GetDeploymentHostPort() (string, int, error) {
	if len(s.Nodes) == 0 {
		return "", 0, errors.New("no nodes found in the active deployment's infrastructure")
	}

	// Prefer node "n11"; if missing, fall back to the first sorted node
	nodeName := "n11"
	node, exists := s.Nodes[nodeName]
	if !exists {
		names := s.ListNodes()
		if len(names) == 0 {
			return "", 0, errors.New("no nodes found in the active deployment's infrastructure")
		}
		nodeName = names[0]
		node = s.Nodes[nodeName]
	}

	// Determine host: prefer DNS name; fall back to public IP
	host := strings.TrimSpace(node.DnsName)
	if host == "" {
		host = strings.TrimSpace(node.PublicIp)
	}
	if host == "" {
		return "", 0, fmt.Errorf(
			"node %s has no reachable host: both dnsName and publicIp are empty",
			nodeName,
		)
	}

	// Parse and validate DB port
	portRaw := strings.TrimSpace(node.Database.DbPort)
	if portRaw == "" {
		return "", 0, errors.New("database port is empty in node details")
	}

	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse database port '%s': %w", portRaw, err)
	}
	if port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("database port out of range: %d", port)
	}

	return host, port, nil
}

// GetCertFingerprint returns the fingerprint (sha256 checksum) of the host certificate.
func (s *NodeDetails) GetCertFingerprint() (string, error) {
	if len(s.Nodes) == 0 {
		return "", errors.New("no nodes found in the active deployment's infrastructure")
	}

	// Prefer node "n11"; if missing, fall back to the first sorted node
	nodeName := "n11"
	node, exists := s.Nodes[nodeName]
	if !exists {
		names := s.ListNodes()
		if len(names) == 0 {
			return "", errors.New("no nodes found in the active deployment's infrastructure")
		}
		nodeName = names[0]
		node = s.Nodes[nodeName]
	}

	// Validate that the TLS certificate is present
	pemStr := strings.TrimSpace(node.TlsCert)
	if pemStr == "" {
		return "", fmt.Errorf("node %s has empty tls certificate", nodeName)
	}

	certDER, _ := pem.Decode([]byte(pemStr))
	if certDER == nil || certDER.Type != "CERTIFICATE" {
		return "", fmt.Errorf(
			"node %s: failed to decode PEM block containing tls certificate",
			nodeName,
		)
	}

	hash := sha256.Sum256(certDER.Bytes)

	return strings.ToUpper(hex.EncodeToString(hash[:])), nil
}
