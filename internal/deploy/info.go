// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

type ConnectionDetails struct {
	Backend         string                    `json:"backend,omitempty"`
	Hostname        string                    `json:"host,omitempty"`
	DisplayHost     string                    `json:"displayHost,omitempty"`
	DBPort          int                       `json:"dbPort,omitempty"`
	AdminUI         *config.DeploymentAdminUI `json:"adminUi,omitempty"`
	Username        string                    `json:"username,omitempty"`
	CertFingerprint string                    `json:"certFingerprint,omitempty"`
	InsecureSkipTLS bool                      `json:"insecureSkipCertValidation,omitempty"`
	SecretsFilePath string                    `json:"-"`
	PublicIp        string                    `json:"publicIp,omitempty"`
	SSHCommand      string                    `json:"sshCommand,omitempty"`
	SSHPort         string                    `json:"sshPort,omitempty"`
	ShellSupported  bool                      `json:"shellSupported,omitempty"`
	AdminUISecured  bool                      `json:"-"`
}

type DeploymentInfoReport struct {
	DeploymentDir   string                   `json:"deploymentDir"`
	DeploymentID    string                   `json:"deploymentId,omitempty"`
	DeploymentState string                   `json:"deploymentState"`
	Presets         *DeploymentPresetSummary `json:"presets,omitempty"`
	Deployment      *config.DeploymentInfo   `json:"deployment,omitempty"`
	Connection      *ConnectionDetails       `json:"connection,omitempty"`
	// Nil for a deployment that cannot run sidecars.
	Sidecars []SidecarSummary `json:"sidecars,omitzero"`
	// SuggestMCP feeds terminal guidance; JSON consumers read Sidecars.
	SuggestMCP bool `json:"-"`
}

type SidecarSummary struct {
	Name      string            `json:"name"`
	Enabled   bool              `json:"enabled"`
	Endpoints []SidecarEndpoint `json:"endpoints"`
}

type DeploymentPresetSummary struct {
	Infrastructure PresetIdentityInfo `json:"infrastructure"`
	Installation   PresetIdentityInfo `json:"installation"`
}

type DeploymentAttributes struct {
	ClusterSize  int    `json:"clusterSize,omitempty"`
	ClusterState string `json:"clusterState,omitempty"`
}

// DeploymentSelection is the flag that selects this deployment in printed
// commands.
func (report *DeploymentInfoReport) DeploymentSelection() string {
	return config.NewDeploymentDir(report.DeploymentDir).CLISelection()
}

// MCPEndpoint is empty unless the MCP server is enabled with a declared endpoint.
func (report *DeploymentInfoReport) MCPEndpoint() string {
	for _, summary := range report.Sidecars {
		if summary.Name != MCPSidecarName || !summary.Enabled {
			continue
		}
		for _, endpoint := range summary.Endpoints {
			if endpoint.URL != "" {
				return endpoint.URL
			}
		}
	}

	return ""
}

func (details *ConnectionDetails) DisplayHostname() string {
	return displayHostname(details)
}

func (details *ConnectionDetails) IsLocalBackend() bool {
	if details == nil {
		return false
	}

	return details.Backend == localDeploymentBackend
}

func (details *ConnectionDetails) HasAdminUI() bool {
	return details != nil && details.AdminUI != nil && details.AdminUI.URL != ""
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
		Backend:         deploymentInfo.Backend,
		Hostname:        connectionInfo.Host,
		DisplayHost:     connectionInfo.DisplayHost,
		PublicIp:        connectionInfo.PublicIP,
		DBPort:          connectionInfo.DBPort,
		AdminUI:         connectionInfo.AdminUI,
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

func GetDeploymentInfoReport(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*DeploymentInfoReport, error) {
	// A successful shared lock gives us a consistent deployment directory snapshot.
	// If another launcher command holds the exclusive lock, we deliberately return
	// only the high-level status instead of reading partially written artifacts.
	var report *DeploymentInfoReport
	err := withDeploymentSharedLock(ctx, deployment, func(deployment config.DeploymentDir) error {
		var getErr error
		report, getErr = getDeploymentInfoReportWithExistingLock(ctx, deployment)

		return getErr
	})
	if err != nil {
		status, statusErr := statusFromLockError(err)
		if statusErr != nil {
			return nil, statusErr
		}

		return minimalDeploymentInfoReport(deployment, status.Status), nil
	}

	return report, nil
}

func getDeploymentInfoReportWithExistingLock(
	ctx context.Context,
	deployment config.DeploymentDir,
) (*DeploymentInfoReport, error) {
	// Callers must already hold either the shared or exclusive deployment directory
	// lock. This avoids lock re-entry when deploy/start generates connection
	// instructions while holding the exclusive lock, and keeps normal user-facing
	// reads protected by GetDeploymentInfoReport's shared lock.
	deploymentStatus, err := GetStatus(ctx, deployment, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	return deploymentInfoReportFromState(ctx, deployment, deploymentStatus.Status)
}

func deploymentInfoReportFromState(
	ctx context.Context,
	deployment config.DeploymentDir,
	deploymentState string,
) (*DeploymentInfoReport, error) {
	report := minimalDeploymentInfoReport(deployment, deploymentState)
	if deploymentState == StatusNotInitialized {
		return report, nil
	}

	if err := addDeploymentIdentity(ctx, deployment, report); err != nil {
		if deploymentState == StatusOperationInProgress {
			return report, nil
		}

		return nil, err
	}
	if deploymentState == StatusOperationInProgress {
		return report, nil
	}
	addDeploymentAttributes(deployment, report)
	report.Sidecars = savedSidecarSummaries(deployment)
	report.SuggestMCP = deploymentState == StatusRunning && SuggestMCPServer(deployment)

	switch deploymentState {
	case StatusRunning:
		connectionDetails, err := readConnectionDetails(deployment)
		if err != nil {
			return nil, err
		}
		report.Connection = connectionDetails

		return report, nil
	default:
		return report, nil
	}
}

func minimalDeploymentInfoReport(
	deployment config.DeploymentDir,
	deploymentState string,
) *DeploymentInfoReport {
	return &DeploymentInfoReport{
		DeploymentDir:   deployment.Root(),
		DeploymentState: deploymentState,
	}
}

func addDeploymentIdentity(
	ctx context.Context,
	deployment config.DeploymentDir,
	report *DeploymentInfoReport,
) error {
	exasolState, err := config.ReadExasolPersonalState(deployment)
	if err != nil {
		return err
	}
	report.DeploymentID = exasolState.DeploymentId
	configuration, err := readDeploymentConfiguration(ctx, deployment)
	if err != nil {
		return err
	}
	report.Presets = &DeploymentPresetSummary{
		Infrastructure: configuration.Infrastructure.Identity,
		Installation:   configuration.Installation.Identity,
	}

	return nil
}

func addDeploymentAttributes(
	deployment config.DeploymentDir,
	report *DeploymentInfoReport,
) {
	if deploymentInfo, err := config.ReadDeploymentInfo(deployment); err == nil {
		if report.DeploymentID == "" {
			report.DeploymentID = deploymentInfo.DeploymentId
		}
		report.Deployment = deploymentInfo
	}
}

// Info reports what the saved definitions declare without inspecting
// containers; the sidecar status commands report what is running.
func savedSidecarSummaries(deployment config.DeploymentDir) []SidecarSummary {
	if !supportsSidecars(deployment) {
		return nil
	}
	document, err := config.ReadSidecars(deployment)
	if err != nil {
		return nil
	}
	summaries := []SidecarSummary{}
	for _, name := range slices.Sorted(maps.Keys(document.Sidecars)) {
		definition := document.Sidecars[name]
		summaries = append(summaries, SidecarSummary{
			Name:      name,
			Enabled:   definition.IsEnabled(),
			Endpoints: savedSidecarEndpoints(definition),
		})
	}

	return summaries
}

func savedSidecarEndpoints(definition sidecar.Definition) []SidecarEndpoint {
	declarations := declarationsFor(definition)
	endpoints := []SidecarEndpoint{}
	for _, port := range definition.Container.Defaults().Ports {
		if port.HostPort > 0 {
			endpoints = append(endpoints, declarations.endpoint(port.HostIP, port.HostPort))
		}
	}

	return endpoints
}

func supportsSidecars(deployment config.DeploymentDir) bool {
	manifest, err := config.ReadInfrastructureManifest(deployment)
	if err != nil {
		return false
	}
	kind, err := resolveBackendKind(manifest)

	return err == nil && kind == backendTypeLocal
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
