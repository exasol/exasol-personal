// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type ConnectionInfo struct {
	Backend                    string
	DeploymentName             string
	DeploymentState            string
	ClusterSize                int
	ClusterState               string
	Host                       string
	DBPort                     int
	UIPort                     int
	Username                   string
	CertFingerprint            string
	InsecureSkipCertValidation bool
	SecretsFilePath            string
	PublicIP                   string
	SSHCommand                 string
	SSHPort                    string
	ShellSupported             bool
}

func ResolveConnectionInfo(deployment DeploymentDir) (*ConnectionInfo, error) {
	if localInfo, err := ReadLocalDeploymentInfo(deployment.Root()); err == nil {
		return resolveLocalConnectionInfo(deployment, localInfo)
	} else if !errors.Is(err, ErrNotLocalDeploymentInfo) &&
		!errors.Is(err, ErrMissingConfigFile) {
		return nil, err
	}

	return resolveCloudConnectionInfo(deployment)
}

func resolveLocalConnectionInfo(
	deployment DeploymentDir,
	localInfo *LocalDeploymentInfo,
) (*ConnectionInfo, error) {
	if localInfo == nil || localInfo.Local == nil {
		return nil, fmt.Errorf("%w: missing local runtime section", ErrNotLocalDeploymentInfo)
	}

	host := strings.TrimSpace(localInfo.Local.Host)
	if host == "" {
		host = "127.0.0.1"
	}

	secretsFilePath, err := SecretsFilePath(deployment)
	if err != nil {
		return nil, err
	}

	username := strings.TrimSpace(localInfo.Local.Username)
	if username == "" {
		username = "sys"
	}

	if localInfo.Local.DBPort <= 0 {
		return nil, errors.New("local deployment database port is missing")
	}
	if localInfo.Local.UIPort <= 0 {
		return nil, errors.New("local deployment UI port is missing")
	}

	return &ConnectionInfo{
		Backend:                    DeploymentBackendLocal,
		DeploymentName:             localInfo.DeploymentID,
		DeploymentState:            localInfo.DeploymentState,
		ClusterSize:                localInfo.ClusterSize,
		ClusterState:               localInfo.ClusterState,
		Host:                       host,
		DBPort:                     localInfo.Local.DBPort,
		UIPort:                     localInfo.Local.UIPort,
		Username:                   username,
		CertFingerprint:            strings.TrimSpace(localInfo.Local.CertFingerprint),
		InsecureSkipCertValidation: localInfo.Local.InsecureSkipCertValidation,
		SecretsFilePath:            secretsFilePath,
		ShellSupported:             false,
	}, nil
}

func resolveCloudConnectionInfo(deployment DeploymentDir) (*ConnectionInfo, error) {
	nodeDetails, err := ReadNodeDetails(deployment)
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

	secretsFilePath, err := SecretsFilePath(deployment)
	if err != nil {
		return nil, err
	}

	return &ConnectionInfo{
		DeploymentName:  nodeDetails.DeploymentId,
		DeploymentState: nodeDetails.DeploymentState,
		ClusterSize:     nodeDetails.ClusterSize,
		ClusterState:    nodeDetails.ClusterState,
		Host:            firstNonEmpty(mainNode.DnsName, mainNode.PublicIp),
		PublicIP:        mainNode.PublicIp,
		DBPort:          parseConnectionPort(mainNode.Database.DbPort),
		UIPort:          parseConnectionPort(mainNode.Database.UiPort),
		Username:        "sys",
		CertFingerprint: certFingerprint,
		SecretsFilePath: secretsFilePath,
		SSHCommand:      mainNode.Ssh.Command,
		SSHPort:         mainNode.Ssh.Port,
		ShellSupported:  true,
	}, nil
}

func parseConnectionPort(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	port, _ := strconv.Atoi(value)

	return port
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}
