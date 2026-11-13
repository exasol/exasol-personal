// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type ConnectionEndpoint struct {
	Node     string `json:"node,omitempty"`
	Host     string `json:"host"`
	DBPort   int    `json:"dbPort"`
	Eligible bool   `json:"eligible"`
}

func ConnectionHosts(endpoints []ConnectionEndpoint, port int) ([]string, error) {
	var hosts []string
	for _, endpoint := range endpoints {
		if !endpoint.Eligible {
			continue
		}
		if endpoint.DBPort != port {
			return nil, fmt.Errorf(
				"multi-node connections require a shared database port: "+
					"node %s uses %d instead of %d", endpoint.Node, endpoint.DBPort, port,
			)
		}
		if !slices.Contains(hosts, endpoint.Host) {
			hosts = append(hosts, endpoint.Host)
		}
	}

	return hosts, nil
}

func deploymentAllowsAllNodes(deployment DeploymentDir, backend string) (bool, error) {
	if backend == "local" {
		return false, nil
	}
	manifest, err := ReadInstallManifest(deployment)
	if errors.Is(err, ErrMissingInstallManifest) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read connection eligibility: %w", err)
	}

	return manifest.DatabaseConnection.AllowAllNodes, nil
}

func (info *DeploymentInfo) connectionEndpoints(allowAllNodes bool) []ConnectionEndpoint {
	if info.Backend == "local" {
		return []ConnectionEndpoint{info.singularConnectionEndpoint()}
	}
	if len(info.Nodes) == 0 {
		endpoint := info.singularConnectionEndpoint()
		if allowAllNodes &&
			(endpoint.Host == "" || endpoint.DBPort <= 0 || endpoint.DBPort > 65535) {
			return []ConnectionEndpoint{}
		}

		return []ConnectionEndpoint{endpoint}
	}
	endpoints := []ConnectionEndpoint{}
	hasEligible := false
	names := info.ListNodes()
	for _, name := range names {
		node := info.Nodes[name]
		host := strings.TrimSpace(node.PublicIp)
		port := parseConnectionPort(node.Database.DbPort)
		if host == "" || port <= 0 || port > 65535 {
			continue
		}
		endpoints = append(endpoints, ConnectionEndpoint{
			Node: name, Host: host, DBPort: port,
			Eligible: allowAllNodes || info.isSingularConnectionNode(node),
		})
		hasEligible = hasEligible || endpoints[len(endpoints)-1].Eligible
	}
	if !allowAllNodes && !hasEligible {
		endpoints = append([]ConnectionEndpoint{info.singularConnectionEndpoint()}, endpoints...)
	}

	return endpoints
}

func (info *DeploymentInfo) isSingularConnectionNode(node DeploymentNode) bool {
	host := strings.TrimSpace(info.Connection.Host)
	return (host == strings.TrimSpace(node.PublicIp) || host == strings.TrimSpace(node.DnsName)) &&
		info.Connection.DBPort == parseConnectionPort(node.Database.DbPort)
}

func (info *DeploymentInfo) singularConnectionEndpoint() ConnectionEndpoint {
	return ConnectionEndpoint{
		Host: strings.TrimSpace(
			info.Connection.Host,
		),
		DBPort:   info.Connection.DBPort,
		Eligible: true,
	}
}
