// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	SQLClientsDocURL = "https://docs.exasol.com/db/latest/connect_exasol/" +
		"sql_clients.htm"
	ProductDocURL                  = "https://docs.exasol.com/"
	connectionInstructionsFileMode = 0o600
)

type ConnectionDetails struct {
	Backend         string
	Hostname        string
	DisplayHost     string
	DBPort          string
	UIPort          string
	Username        string
	CertFingerprint string
	InsecureSkipTLS bool
	SecretsFilePath string
	DeploymentName  string
	PublicIp        string
	SSHCommand      string
	SSHPort         string
	ClusterState    string
	ClusterSize     int
	ShellSupported  bool
	AdminUISecured  bool
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

func getConnectionDetails(deployment config.DeploymentDir) (*ConnectionDetails, error) {
	deploymentInfo, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment info: %w", err)
	}
	connectionInfo, err := deploymentInfo.ConnectionInfo(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve deployment connection info: %w", err)
	}
	secrets, err := config.ReadSecrets(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment secrets: %w", err)
	}

	return &ConnectionDetails{
		Backend:         deploymentInfo.Backend,
		DeploymentName:  connectionInfo.DeploymentName,
		ClusterSize:     connectionInfo.ClusterSize,
		ClusterState:    connectionInfo.ClusterState,
		Hostname:        connectionInfo.Host,
		DisplayHost:     connectionInfo.DisplayHost,
		PublicIp:        connectionInfo.PublicIP,
		DBPort:          strconv.Itoa(connectionInfo.DBPort),
		UIPort:          strconv.Itoa(connectionInfo.UIPort),
		SSHCommand:      connectionInfo.SSHCommand,
		SSHPort:         connectionInfo.SSHPort,
		Username:        connectionInfo.Username,
		CertFingerprint: connectionInfo.CertFingerprint,
		InsecureSkipTLS: connectionInfo.InsecureSkipCertValidation,
		SecretsFilePath: connectionInfo.SecretsFilePath,
		ShellSupported:  connectionInfo.ShellSupported,
		AdminUISecured:  secrets.AdminUiPassword != "",
	}, nil
}

func GetSQLInstructions(connectionDetails *ConnectionDetails) string {
	uiURL := "https://" + net.JoinHostPort(
		displayHostname(connectionDetails),
		connectionDetails.UIPort,
	)
	certificateLine := ""
	if connectionDetails.CertFingerprint != "" {
		certificateLine = "  - Certificate Fingerprint: " + connectionDetails.CertFingerprint + "\n"
	} else if connectionDetails.InsecureSkipTLS {
		certificateLine = "  - Certificate Validation: disable validation / " +
			"use nocertcheck for the current deployment setup\n"
	}

	adminUIInstructions := `
=== How to Connect from a Graphical SQL Client ===
To connect using a client of your choice:
1. Create a new database connection.
2. Choose 'Exasol' as the driver.
3. Enter the following values below in 'Database':
  - Server: ` + displayHostname(connectionDetails) + `
  - Port: ` + connectionDetails.DBPort + `
  - UserId: ` + connectionDetails.Username + `
` + certificateLine + `  - Password: <stored in ` + connectionDetails.SecretsFilePath + `>

=== CLI Connection Instructions ===
To connect using the CLI:
  exasol connect

=== How to open the Administration UI ===
1. Open the following URL in the browser: ` + uiURL + `
2. Accept certificate if necessary

`
	adminUIUsername := ""
	switch {
	case connectionDetails.Backend == config.DeploymentBackendLocal:
		adminUIUsername = connectionDetails.Username
	case connectionDetails.AdminUISecured:
		adminUIUsername = "admin"
	default:
	}

	if adminUIUsername != "" {
		adminUIInstructions += fmt.Sprintf(
			"3. Login with username %q and password stored in %s\n\n",
			adminUIUsername,
			connectionDetails.SecretsFilePath,
		)
	}

	instructions := `

` + adminUIInstructions

	if !connectionDetails.ShellSupported {
		return instructions
	}

	return instructions + `=== SSH Connection Instructions ===
  Public IP: ` + connectionDetails.PublicIp + `
  Primary admin shell (COS): exasol shell container
  Host shell (OS): exasol shell host
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

func GetConnectionInstructionsText(
	ctx context.Context,
	deployment config.DeploymentDir,
) (string, error) {
	var content string
	err := withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		var getErr error
		content, getErr = getConnectionInstructionsTextUnsafe(ctx, deployment)

		return getErr
	})
	if err != nil {
		return "", err
	}

	return content, nil
}

func getConnectionInstructionsTextUnsafe(
	ctx context.Context,
	deployment config.DeploymentDir,
) (string, error) {
	deploymentStatus, err := GetStatus(ctx, deployment, false)
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	wfState := deploymentStatus.Status

	switch wfState {
	case StatusRunning:
		connectionDetails, err := getConnectionDetails(deployment)
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
		connectionDetails, err := getConnectionDetails(deployment)
		if err != nil {
			return "", err
		}
		content := GetHeader(connectionDetails, wfState) + GetDocumentationLink()

		return content, nil
	default:
		return deploymentStatus.Message, nil
	}
}

func writeConnectionInstructionsFile(deployment config.DeploymentDir, content string) error {
	err := os.WriteFile(
		deployment.ConnectionInstructionsPath(),
		[]byte(content),
		connectionInstructionsFileMode,
	)
	if err != nil {
		return fmt.Errorf("failed to write instructions to file: %w", err)
	}

	return nil
}

func PrintConnectionInsInJson(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	return withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		return printConnectionInsInJSONUnsafe(ctx, deployment, writer)
	})
}

func printConnectionInsInJSONUnsafe(
	ctx context.Context,
	deployment config.DeploymentDir,
	writer io.Writer,
) error {
	deploymentStatus, err := GetStatus(ctx, deployment, false)
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
	if wfState == StatusRunning || wfState == StatusStopped {
		info, err := config.ReadDeploymentInfo(deployment)
		if err != nil {
			return err
		}
		info.DeploymentState = wfState

		return encoder.Encode(info)
	}

	return encoder.Encode(details)
}

func displayHostname(connectionDetails *ConnectionDetails) string {
	if connectionDetails == nil {
		return ""
	}
	if connectionDetails.DisplayHost != "" {
		return connectionDetails.DisplayHost
	}
	if connectionDetails.Hostname != "" {
		return connectionDetails.Hostname
	}
	if connectionDetails.PublicIp != "" {
		return connectionDetails.PublicIp
	}

	return connectionDetails.Hostname
}
