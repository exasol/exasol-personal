// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/config"
)

const (
	SQLClientsDocURL = "https://docs.exasol.com/db/latest/connect_exasol/" +
		"sql_clients.htm"
	ProductDocURL                  = "https://docs.exasol.com/"
	connectionInstructionsFileMode = 0o600
)

type ConnectionDetails struct {
	DeploymentOverview

	Backend         string
	Hostname        string
	DisplayHost     string
	DBPort          string
	AdminUI         *config.DeploymentAdminUI
	AILab           *config.DeploymentAILab
	Username        string
	CertFingerprint string
	InsecureSkipTLS bool
	SecretsFilePath string
	PublicIp        string
	SSHCommand      string
	SSHPort         string
	ShellSupported  bool
	AdminUISecured  bool
	AILabSecured    bool
}

type DeploymentOverview struct {
	DeploymentName  string
	DeploymentState string
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

func readConnectionDetails(deployment config.DeploymentDir) (*ConnectionDetails, error) {
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
		DeploymentOverview: DeploymentOverview{
			DeploymentName: connectionInfo.DeploymentName,
			ClusterSize:    connectionInfo.ClusterSize,
			ClusterState:   connectionInfo.ClusterState,
		},
		Backend:         deploymentInfo.Backend,
		Hostname:        connectionInfo.Host,
		DisplayHost:     connectionInfo.DisplayHost,
		PublicIp:        connectionInfo.PublicIP,
		DBPort:          strconv.Itoa(connectionInfo.DBPort),
		AdminUI:         connectionInfo.AdminUI,
		AILab:           connectionInfo.AILab,
		SSHCommand:      connectionInfo.SSHCommand,
		SSHPort:         connectionInfo.SSHPort,
		Username:        connectionInfo.Username,
		CertFingerprint: connectionInfo.CertFingerprint,
		InsecureSkipTLS: connectionInfo.InsecureSkipCertValidation,
		SecretsFilePath: connectionInfo.SecretsFilePath,
		ShellSupported:  connectionInfo.ShellSupported,
		AdminUISecured:  secrets.AdminUiPassword != "",
		AILabSecured:    secrets.AiLabJupyterPassword != "",
	}, nil
}

func GetSQLInstructions(connectionDetails *ConnectionDetails) string {
	certificateLine := ""
	if connectionDetails.CertFingerprint != "" {
		certificateLine = "  - Certificate Fingerprint: " + connectionDetails.CertFingerprint + "\n"
	} else if connectionDetails.InsecureSkipTLS {
		certificateLine = "  - Certificate Validation: disable validation / " +
			"use nocertcheck for the current deployment setup\n"
	}

	instructions := `
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

`

	instructions += getAdminUIInstructions(connectionDetails)
	instructions += getAILabInstructions(connectionDetails)

	if !connectionDetails.ShellSupported {
		return instructions
	}

	if connectionDetails.Backend == localDeploymentBackend {
		return instructions + `=== Local Shell Instructions ===
  Local endpoint: ` + displayHostname(connectionDetails) + `
  Exasol Local database container shell: exasol shell container
  Host shell (VM OS): exasol shell host
  Alternative: ` + connectionDetails.SSHCommand + `

Note: exasol destroy deletes the local VM disk/data and launcher-managed share for this deployment.

`
	}

	return instructions + `=== SSH Connection Instructions ===
  Public IP: ` + connectionDetails.PublicIp + `
  Primary admin shell (COS): exasol shell container
  Host shell (OS): exasol shell host
  Alternative: ` + connectionDetails.SSHCommand + `

`
}

func getAdminUIInstructions(connectionDetails *ConnectionDetails) string {
	if connectionDetails == nil || connectionDetails.AdminUI == nil ||
		connectionDetails.AdminUI.URL == "" {
		return ""
	}

	instructions := `
=== How to open the Administration UI ===
  URL: ` + connectionDetails.AdminUI.URL + `
`
	if connectionDetails.AdminUI.Username != "" {
		instructions += "  Username: " + connectionDetails.AdminUI.Username + "\n"
	}
	if connectionDetails.AdminUISecured {
		instructions += "  Password: <stored in " + connectionDetails.SecretsFilePath + ">\n"
	}
	if connectionDetails.AdminUI.CertFingerprint != "" {
		instructions += "  Certificate Fingerprint: " +
			connectionDetails.AdminUI.CertFingerprint + "\n"
	} else if connectionDetails.AdminUI.InsecureSkipCertValidation {
		instructions += "  Certificate Validation: accept the certificate if necessary\n"
	}

	return instructions + "\n"
}

func getAILabInstructions(connectionDetails *ConnectionDetails) string {
	if connectionDetails == nil || connectionDetails.AILab == nil ||
		connectionDetails.AILab.URL == "" {
		return ""
	}

	instructions := `
=== How to open the AI Lab ===
  URL: ` + connectionDetails.AILab.URL + `
`
	if connectionDetails.AILabSecured {
		secretsRef := "<stored in " + connectionDetails.SecretsFilePath + ">"
		instructions += "  Jupyter password: " + secretsRef + "\n"
		instructions += "  Config-store master password: " + secretsRef + "\n"
	}
	instructions += "  The database and BucketFS connections are pre-configured.\n"

	return instructions + "\n"
}

func GetDocumentationLink() string {
	return fmt.Sprintf(`
=== SQL clients documentation ===
  %s
=== Exasol Product Documentation ===
Or visit %s for general information about Exasol products.
`, SQLClientsDocURL, ProductDocURL)
}

func renderDeploymentOverview(overview DeploymentOverview) string {
	return `
Exasol Personal Deployment Overview
Deployment Name: ` + overview.DeploymentName + `
Deployment State: ` + overview.DeploymentState + `
Cluster Size: ` + strconv.Itoa(overview.ClusterSize) + `
Cluster State: ` + overview.ClusterState + `
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
		connectionDetails, err := readConnectionDetails(deployment)
		if err != nil {
			return "", err
		}
		overview := connectionDetails.DeploymentOverview
		overview.DeploymentState = wfState
		content := renderDeploymentOverview(
			overview,
		) + GetSQLInstructions(
			connectionDetails,
		) + GetDocumentationLink()

		return content, nil
	case StatusStopped:
		overview, err := readDeploymentOverview(deployment, wfState)
		if err != nil {
			return "", err
		}
		content := renderDeploymentOverview(overview) + GetDocumentationLink()

		return content, nil
	default:
		return deploymentStatus.Message, nil
	}
}

func readDeploymentOverview(
	deployment config.DeploymentDir,
	wfState string,
) (DeploymentOverview, error) {
	deploymentInfo, err := config.ReadDeploymentInfo(deployment)
	if err != nil {
		return DeploymentOverview{}, fmt.Errorf("failed to read deployment info: %w", err)
	}

	clusterState := strings.TrimSpace(deploymentInfo.ClusterState)
	if clusterState == "" {
		clusterState = wfState
	}

	return DeploymentOverview{
		DeploymentName:  deploymentInfo.DeploymentId,
		DeploymentState: wfState,
		ClusterSize:     deploymentInfo.ClusterSize,
		ClusterState:    clusterState,
	}, nil
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
