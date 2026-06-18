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
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	nodeDetailsFileName    = "deployment.json"
	ConnectionInstruction  = "connection-instructions.txt"
	primaryNodeName        = "n11"
	defaultUsername        = "sys"
	defaultAdminUIUsername = "admin"
)

type DeploymentInfo struct {
	Backend          string                    `json:"backend,omitempty"`
	DeploymentId     string                    `json:"deploymentId"`
	DeploymentState  string                    `json:"deploymentState,omitempty"`
	Region           string                    `json:"region,omitempty"`
	AvailabilityZone string                    `json:"availabilityZone,omitempty"`
	ClusterSize      int                       `json:"clusterSize,omitempty"`
	ClusterState     string                    `json:"clusterState,omitempty"`
	InstanceType     string                    `json:"instanceType,omitempty"`
	VpcId            string                    `json:"vpcId,omitempty"`
	SubnetId         string                    `json:"subnetId,omitempty"`
	Nodes            map[string]DeploymentNode `json:"nodes,omitempty"`
	Connection       *DeploymentConnection     `json:"connection,omitempty"`
	SaaS             *DeploymentSaaS           `json:"saas,omitempty"`
}

// DeploymentSaaS caches the non-secret SaaS migration target resolved from the
// SaaS API, so connection and migration commands do not have to re-query it.
type DeploymentSaaS struct {
	AccountId string `json:"accountId,omitempty"`
	Region    string `json:"region,omitempty"`
	DbUuid    string `json:"dbUuid,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	JDBC      string `json:"jdbc,omitempty"`
	Username  string `json:"username,omitempty"`
}

type DeploymentNode struct {
	AvailabilityZone string             `json:"availabilityZone,omitempty"`
	Database         DeploymentDatabase `json:"database,omitempty"`
	DnsName          string             `json:"dnsName,omitempty"`
	InstanceId       string             `json:"instanceId,omitempty"`
	PrivateIp        string             `json:"privateIp,omitempty"`
	PublicIp         string             `json:"publicIp,omitempty"`
	Ssh              DeploymentSSH      `json:"ssh,omitempty"`
	TlsCert          string             `json:"tlsCert,omitempty"`
}

type DeploymentDatabase struct {
	DbPort string `json:"dbPort,omitempty"`
	UiPort string `json:"uiPort,omitempty"`
	Url    string `json:"url,omitempty"`
}

type DeploymentSSH struct {
	Command  string `json:"command,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
	KeyName  string `json:"keyName,omitempty"`
	Port     string `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
}

type DeploymentConnection struct {
	Host                       string             `json:"host,omitempty"`
	DisplayHost                string             `json:"displayHost,omitempty"`
	PublicIp                   string             `json:"publicIp,omitempty"`
	DBPort                     int                `json:"dbPort,omitempty"`
	UIPort                     int                `json:"uiPort,omitempty"`
	AdminUI                    *DeploymentAdminUI `json:"adminUi,omitempty"`
	Username                   string             `json:"username,omitempty"`
	CertFingerprint            string             `json:"certFingerprint,omitempty"`
	InsecureSkipCertValidation bool               `json:"insecureSkipCertValidation,omitempty"`
	SSHCommand                 string             `json:"sshCommand,omitempty"`
	SSHPort                    string             `json:"sshPort,omitempty"`
	ShellSupported             bool               `json:"shellSupported,omitempty"`
}

type DeploymentAdminUI struct {
	URL                        string `json:"url,omitempty"`
	Username                   string `json:"username,omitempty"`
	CertFingerprint            string `json:"certFingerprint,omitempty"`
	InsecureSkipCertValidation bool   `json:"insecureSkipCertValidation,omitempty"`
}

type (
	NodeDetails              = DeploymentInfo
	nodeDetailsNode          = DeploymentNode
	nodeDetailsNodesDatabase = DeploymentDatabase
)

type SSHDetails struct {
	Host    string
	Port    string
	User    string
	KeyFile string
}

func ReadDeploymentInfo(deployment DeploymentDir) (*DeploymentInfo, error) {
	deploymentInfoPath, exists, err := findExistingFile(deployment.Root(), nodeDetailsFileName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf(
			"deployment info file not found in deployment directory: expected %q in %s",
			nodeDetailsFileName,
			deployment.Root(),
		)
	}

	slog.Debug("reading deployment info file", "file", deploymentInfoPath)

	info, err := readConfig[DeploymentInfo](deploymentInfoPath, "deployment info")
	if err != nil {
		return nil, err
	}

	normalizeDeploymentInfo(info)
	if !hasRecognizedDeploymentInfo(info) {
		return nil, errors.New("deployment info file has no recognizable schema")
	}

	return info, nil
}

func ReadNodeDetails(deployment DeploymentDir) (*NodeDetails, error) {
	return ReadDeploymentInfo(deployment)
}

func WriteDeploymentInfo(deploymentDir string, info *DeploymentInfo) error {
	if info == nil {
		return errors.New("deployment info is required")
	}

	normalized := *info
	normalizeDeploymentInfo(&normalized)

	return writeConfig(
		&normalized,
		filepath.Join(deploymentDir, nodeDetailsFileName),
		"deployment info",
	)
}

func GetDeploymentInfoFilePath(deploymentDir string) (string, bool, error) {
	return findExistingFile(deploymentDir, nodeDetailsFileName)
}

var ErrNoNodeDetailsFile = errors.New("node details file not found in deployment directory")

// Returns the list of node names, sorted increasingly.
func (s *DeploymentInfo) ListNodes() []string {
	result := make([]string, 0, len(s.Nodes))
	for nodeName := range s.Nodes {
		result = append(result, nodeName)
	}

	sort.Strings(result)

	return result
}

var ErrUnknownNodeName = errors.New("unknown node name")

func (s *DeploymentInfo) GetSSHDetails(
	nodeName string,
	deployment DeploymentDir,
) (*SSHDetails, error) {
	entry, exists := s.Nodes[nodeName]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNodeName, nodeName)
	}

	return &SSHDetails{
		Host:    entry.PublicIp,
		User:    entry.Ssh.Username,
		KeyFile: deployment.Resolve(entry.Ssh.KeyFile),
		Port:    entry.Ssh.Port,
	}, nil
}

// GetDeploymentHostPort returns the host-port string of the primary database endpoint.
func (s *DeploymentInfo) GetDeploymentHostPort() (string, int, error) {
	if s.Connection != nil && strings.TrimSpace(s.Connection.Host) != "" &&
		s.Connection.DBPort > 0 {
		return s.Connection.Host, s.Connection.DBPort, nil
	}

	if len(s.Nodes) == 0 {
		return "", 0, errors.New("no nodes found in the active deployment's infrastructure")
	}

	// Prefer node "n11"; if missing, fall back to the first sorted node
	nodeName := primaryNodeName
	node, exists := s.Nodes[nodeName]
	if !exists {
		names := s.ListNodes()
		if len(names) == 0 {
			return "", 0, errors.New("no nodes found in the active deployment's infrastructure")
		}
		nodeName = names[0]
		node = s.Nodes[nodeName]
	}

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
func (s *DeploymentInfo) GetCertFingerprint() (string, error) {
	if s.Connection != nil && strings.TrimSpace(s.Connection.CertFingerprint) != "" {
		return strings.TrimSpace(s.Connection.CertFingerprint), nil
	}

	if len(s.Nodes) == 0 {
		return "", errors.New("no nodes found in the active deployment's infrastructure")
	}

	nodeName := primaryNodeName
	node, exists := s.Nodes[nodeName]
	if !exists {
		names := s.ListNodes()
		if len(names) == 0 {
			return "", errors.New("no nodes found in the active deployment's infrastructure")
		}
		nodeName = names[0]
		node = s.Nodes[nodeName]
	}

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

func normalizeDeploymentInfo(info *DeploymentInfo) {
	if info == nil {
		return
	}

	info.Backend = strings.TrimSpace(info.Backend)

	if info.Connection == nil && len(info.Nodes) > 0 {
		info.Connection = deriveConnectionFromNodes(info)
	}

	if info.Connection != nil {
		if strings.TrimSpace(info.Connection.DisplayHost) == "" {
			info.Connection.DisplayHost = firstNonEmpty(
				info.Connection.DisplayHost,
				info.Connection.Host,
				info.Connection.PublicIp,
			)
		}
		if strings.TrimSpace(info.Connection.Host) == "" {
			info.Connection.Host = firstNonEmpty(
				info.Connection.Host,
				info.Connection.PublicIp,
			)
		}
		if strings.TrimSpace(info.Connection.Username) == "" {
			info.Connection.Username = defaultUsername
		}
		normalizeAdminUIConnection(info.Connection)
	}

	if info.Backend == "" {
		if len(info.Nodes) > 0 || info.Region != "" || info.VpcId != "" || info.SubnetId != "" {
			info.Backend = "tofu"
		}
	}
}

func hasRecognizedDeploymentInfo(info *DeploymentInfo) bool {
	if info == nil {
		return false
	}

	return strings.TrimSpace(info.DeploymentId) != "" ||
		strings.TrimSpace(info.Backend) != "" ||
		info.Connection != nil ||
		len(info.Nodes) > 0 ||
		strings.TrimSpace(info.Region) != "" ||
		strings.TrimSpace(info.VpcId) != "" ||
		strings.TrimSpace(info.SubnetId) != ""
}

func deriveConnectionFromNodes(info *DeploymentInfo) *DeploymentConnection {
	if info == nil || len(info.Nodes) == 0 {
		return nil
	}

	nodes := info.ListNodes()
	if len(nodes) == 0 {
		return nil
	}

	primaryNodeName := primaryNodeName
	if _, ok := info.Nodes[primaryNodeName]; !ok {
		primaryNodeName = nodes[0]
	}
	mainNode := info.Nodes[primaryNodeName]
	host := firstNonEmpty(mainNode.DnsName, mainNode.PublicIp)

	connection := &DeploymentConnection{
		Host:           host,
		DisplayHost:    host,
		PublicIp:       mainNode.PublicIp,
		DBPort:         parseConnectionPort(mainNode.Database.DbPort),
		UIPort:         parseConnectionPort(mainNode.Database.UiPort),
		Username:       defaultUsername,
		SSHCommand:     mainNode.Ssh.Command,
		SSHPort:        mainNode.Ssh.Port,
		ShellSupported: true,
	}
	if adminUI := deriveAdminUIFromNode(mainNode, host); adminUI != nil {
		connection.AdminUI = adminUI
	}

	if certFingerprint, err := info.GetCertFingerprint(); err == nil {
		connection.CertFingerprint = certFingerprint
	}

	return connection
}

func normalizeAdminUIConnection(connection *DeploymentConnection) {
	if connection == nil {
		return
	}

	if connection.AdminUI == nil && connection.UIPort > 0 {
		connection.AdminUI = &DeploymentAdminUI{
			URL: makeAdminUIURL(
				firstNonEmpty(connection.DisplayHost, connection.Host, connection.PublicIp),
				connection.UIPort,
			),
			Username: defaultAdminUIUsername,
		}
	}

	if connection.AdminUI == nil {
		return
	}

	connection.AdminUI.URL = strings.TrimSpace(connection.AdminUI.URL)
	connection.AdminUI.Username = strings.TrimSpace(connection.AdminUI.Username)
	connection.AdminUI.CertFingerprint = strings.TrimSpace(connection.AdminUI.CertFingerprint)
	if connection.AdminUI.URL == "" {
		connection.AdminUI = nil
		return
	}
	if connection.AdminUI.Username == "" {
		connection.AdminUI.Username = defaultAdminUIUsername
	}
}

func deriveAdminUIFromNode(node DeploymentNode, host string) *DeploymentAdminUI {
	url := strings.TrimSpace(node.Database.Url)
	if url == "" {
		uiPort := parseConnectionPort(node.Database.UiPort)
		if uiPort <= 0 {
			return nil
		}
		url = makeAdminUIURL(host, uiPort)
	}
	if url == "" {
		return nil
	}

	return &DeploymentAdminUI{
		URL:      url,
		Username: defaultAdminUIUsername,
	}
}

func makeAdminUIURL(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 {
		return ""
	}

	return "https://" + net.JoinHostPort(host, strconv.Itoa(port))
}
