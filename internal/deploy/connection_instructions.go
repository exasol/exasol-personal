// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	SQLClientsDocURL = "https://docs.exasol.com/db/latest/connect_exasol/sql_clients.htm"
	ProductDocURL    = "https://docs.exasol.com/"
)

type ConnectionDetails struct {
	Hostname        string
	DBPort          string
	UIPort          string
	Username        string
	CertFingerprint string
	SecretsFilePath string
	DeploymentName  string
	PublicIp        string
	SSHCommand      string
	SSHPort         string
	ClusterState    string
	ClusterSize     int
}

type DocumentationLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Details struct {
	DeploymentID    string              `json:"deploymentId,omitempty"`
	DeploymentState string              `json:"deploymentState"`
	ClusterSize     int                 `json:"clusterSize,omitempty"`
	ClusterState    string              `json:"clusterstate,omitempty"`
	Documentation   []DocumentationLink `json:"documentation"`
}

func getConnectionDetails(deploymentDir string) (*ConnectionDetails, error) {
	nodeDetails, err := config.ReadNodeDetails(deploymentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get node details: %w", err)
	}

	certFingerprint, err := nodeDetails.GetCertFingerprint()
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment tls certificate: %w", err)
	}

	nodes := nodeDetails.ListNodes()
	if len(nodes) == 0 {
		return nil, errors.New("no nodes found in deployment")
	}
	mainNode := nodeDetails.Nodes[nodes[0]]

	secretsFilePath, err := config.GetSecretsFilePath(deploymentDir)
	if err != nil {
		return nil, err
	}

	return &ConnectionDetails{
		DeploymentName:  nodeDetails.DeploymentId,
		ClusterSize:     nodeDetails.ClusterSize,
		ClusterState:    nodeDetails.ClusterState,
		Hostname:        mainNode.DnsName,
		PublicIp:        mainNode.PublicIp,
		DBPort:          mainNode.Database.DbPort,
		UIPort:          mainNode.Database.UiPort,
		SSHCommand:      mainNode.Ssh.Command,
		SSHPort:         mainNode.Ssh.Port,
		Username:        "sys",
		CertFingerprint: certFingerprint,
		SecretsFilePath: secretsFilePath,
	}, nil
}

func GetSQLInstructions(connectionDetails *ConnectionDetails) string {
	uiURL := "https://" + net.JoinHostPort(connectionDetails.Hostname, connectionDetails.UIPort)

	return `
=== How to Connect from a Graphical SQL Client ===
To connect using a client of your choice:
1. Create a new database connection.
2. Choose 'Exasol' as the driver.
3. Enter the following values below in 'Database':
  - Server: ` + connectionDetails.Hostname + `
  - Port: ` + connectionDetails.DBPort + `
  - UserId: ` + connectionDetails.Username + `
  - Certificate Fingerprint: ` + connectionDetails.CertFingerprint + `
  - Password: <stored in ` + connectionDetails.SecretsFilePath + `>

=== CLI Connection Instructions ===
To connect using the CLI:
  exasol connect

=== How to open the Administration UI ===
1. Open the following URL in the browser: ` + uiURL + `
2. Accept certificate if necessary
3. Login with username "admin" and password stored in ` + connectionDetails.SecretsFilePath + `

=== SSH Connection Instructions ===
  Public IP: ` + connectionDetails.PublicIp + `
  Preferred: exasol diag shell
  Alternative: ` + connectionDetails.SSHCommand + `

`
}

func GetDocumentationLink() string {
	return fmt.Sprintf(`
=== SQL clients documentation ===
  %s
=== Exasol Product Documentation ===
Or visit %s for general information about Exasol products.
`, SQLClientsDocURL, ProductDocURL)
}

func GetHeader(connectionDetails *ConnectionDetails, wfState string) string {
	return `
Exasol Personal Deployment Overview
Deployment Name: ` + connectionDetails.DeploymentName + `
Deployment State: ` + wfState + `
Cluster Size: ` + strconv.Itoa(connectionDetails.ClusterSize) + `
Cluster State: ` + connectionDetails.ClusterState + `
`
}

func GetConnectionInstructionsText(ctx context.Context, deploymentDir string) (string, error) {
	var content string
	err := withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		var getErr error
		content, getErr = getConnectionInstructionsTextUnsafe(ctx, dir)

		return getErr
	})
	if err != nil {
		return "", err
	}

	return content, nil
}

func getConnectionInstructionsTextUnsafe(
	ctx context.Context,
	deploymentDir string,
) (string, error) {
	deploymentStatus, err := GetStatus(ctx, deploymentDir, false)
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	wfState := deploymentStatus.Status

	switch wfState {
	case StatusRunning:
		connectionDetails, err := getConnectionDetails(deploymentDir)
		if err != nil {
			return "", err
		}
		content := GetHeader(
			connectionDetails,
			wfState,
		) + GetSQLInstructions(
			connectionDetails,
		) + GetDocumentationLink()

		return content, nil
	case StatusStopped:
		connectionDetails, err := getConnectionDetails(deploymentDir)
		if err != nil {
			return "", err
		}
		content := GetHeader(connectionDetails, wfState) + GetDocumentationLink()

		return content, nil
	default:
		return deploymentStatus.Message, nil
	}
}

func writeConnectionInstructionsFile(deploymentDir string, content string) error {
	connInsFile, err := os.Create(filepath.Join(deploymentDir, config.ConnectionInstruction))
	if err != nil {
		return fmt.Errorf("failed to create instructions file: %w", err)
	}

	defer connInsFile.Close()

	_, err = connInsFile.WriteString(content)
	if err != nil {
		return fmt.Errorf("failed to write instructions to file: %w", err)
	}

	return nil
}

func PrintConnectionInsInJson(ctx context.Context, deploymentDir string, writer io.Writer) error {
	return withDeploymentSharedLock(ctx, deploymentDir, func(dir string) error {
		return printConnectionInsInJSONUnsafe(ctx, dir, writer)
	})
}

func printConnectionInsInJSONUnsafe(
	ctx context.Context,
	deploymentDir string,
	writer io.Writer,
) error {
	deploymentStatus, err := GetStatus(ctx, deploymentDir, false)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	wfState := deploymentStatus.Status

	details := Details{}
	details.DeploymentState = wfState
	details.Documentation = []DocumentationLink{
		{
			Title: "SQL clients documentation",
			URL:   SQLClientsDocURL,
		},
		{
			Title: "Exasol Product Documentation",
			URL:   ProductDocURL,
		},
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	switch wfState {
	case StatusRunning:
		nodeDetails, err := config.ReadNodeDetails(deploymentDir)
		nodeDetails.DeploymentState = wfState
		if err != nil {
			return err
		}

		return encoder.Encode(nodeDetails)
	case StatusStopped:
		connectionDetails, err := getConnectionDetails(deploymentDir)
		if err != nil {
			return err
		}
		details.DeploymentID = connectionDetails.DeploymentName
		details.ClusterSize = connectionDetails.ClusterSize
		details.ClusterState = connectionDetails.ClusterState

		return encoder.Encode(details)
	default:
		return encoder.Encode(details)
	}
}
