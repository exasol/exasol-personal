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
	DeploymentName             string
	DeploymentState            string
	ClusterSize                int
	ClusterState               string
	Host                       string
	DisplayHost                string
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
	info, err := ReadDeploymentInfo(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment info: %w", err)
	}

	return info.ConnectionInfo(deployment)
}

func (info *DeploymentInfo) ConnectionInfo(deployment DeploymentDir) (*ConnectionInfo, error) {
	if info == nil {
		return nil, errors.New("deployment info is required")
	}
	if info.Connection == nil {
		return nil, errors.New("deployment connection details are missing")
	}

	secretsFilePath, err := SecretsFilePath(deployment)
	if err != nil {
		return nil, err
	}

	host := strings.TrimSpace(info.Connection.Host)
	if host == "" {
		return nil, errors.New("deployment connection host is missing")
	}
	if info.Connection.DBPort <= 0 {
		return nil, errors.New("deployment database port is missing")
	}
	if info.Connection.UIPort <= 0 {
		return nil, errors.New("deployment UI port is missing")
	}

	displayHost := strings.TrimSpace(info.Connection.DisplayHost)
	if displayHost == "" {
		displayHost = host
	}

	username := strings.TrimSpace(info.Connection.Username)
	if username == "" {
		username = "sys"
	}

	certFingerPrint := resolveConnectionCertFingerprint(info)

	return &ConnectionInfo{
		DeploymentName:             info.DeploymentId,
		DeploymentState:            info.DeploymentState,
		ClusterSize:                info.ClusterSize,
		ClusterState:               info.ClusterState,
		Host:                       host,
		DisplayHost:                displayHost,
		DBPort:                     info.Connection.DBPort,
		UIPort:                     info.Connection.UIPort,
		Username:                   username,
		CertFingerprint:            certFingerPrint,
		InsecureSkipCertValidation: info.Connection.InsecureSkipCertValidation,
		SecretsFilePath:            secretsFilePath,
		PublicIP:                   strings.TrimSpace(info.Connection.PublicIp),
		SSHCommand:                 strings.TrimSpace(info.Connection.SSHCommand),
		SSHPort:                    strings.TrimSpace(info.Connection.SSHPort),
		ShellSupported:             info.Connection.ShellSupported,
	}, nil
}

func resolveConnectionCertFingerprint(info *DeploymentInfo) string {
	if info == nil || info.Connection == nil {
		return ""
	}
	if fingerPrint := strings.TrimSpace(info.Connection.CertFingerprint); fingerPrint != "" {
		return fingerPrint
	}

	return legacyNodeTLSCertFingerprintFallback(info)
}

// legacyNodeTLSCertFingerprintFallback preserves TLS fingerprint resolution for deployment.json
// files that still only contain nodes[*].tlsCert. Keep this fallback isolated so it can be
// removed when legacy node-derived connection metadata is no longer supported.
func legacyNodeTLSCertFingerprintFallback(info *DeploymentInfo) string {
	certFingerprint, err := info.GetCertFingerprint()
	if err != nil {
		return ""
	}

	return certFingerprint
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
