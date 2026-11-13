// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
)

func TestConnectionInstructionsAdvertiseOnlyEligibleMultiHostConnections(t *testing.T) {
	t.Parallel()
	for _, allowAllNodes := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "legacy", true: "unlimited"}[allowAllNodes],
			func(t *testing.T) {
				t.Parallel()
				// Given
				report := &DeploymentInfoReport{
					Connection: &ConnectionDetails{
						Hostname: "primary.example", DBPort: 8563,
						Endpoints: []config.ConnectionEndpoint{
							{Host: "203.0.113.11", DBPort: 8563, Eligible: true},
							{Host: "203.0.113.12", DBPort: 8563, Eligible: allowAllNodes},
						},
					},
				}
				// When
				content, err := RenderConnectionInstructionsText(report)
				// Then
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(
					content,
					"Multi-host server list: 203.0.113.11,203.0.113.12",
				) != allowAllNodes {
					t.Fatalf("instructions advertise the wrong endpoints: %s", content)
				}
			},
		)
	}
}

func TestMultiHostInstructionsRequireSharedPort(t *testing.T) {
	t.Parallel()
	// Given
	details := &ConnectionDetails{DBPort: 8563, Endpoints: []config.ConnectionEndpoint{
		{Host: "203.0.113.11", DBPort: 8563, Eligible: true},
		{Host: "203.0.113.12", DBPort: 9000, Eligible: true},
	}}
	// When
	hosts := details.MultiHost()
	// Then
	if hosts != "" {
		t.Fatalf("a shared-port host list cannot represent these endpoints: %s", hosts)
	}
}

func TestConnectionInstructionsUseAnExposedSecondaryEndpoint(t *testing.T) {
	t.Parallel()
	// Given
	report := &DeploymentInfoReport{Connection: &ConnectionDetails{
		Hostname: "private-primary.example", DBPort: 8563,
		Endpoints: []config.ConnectionEndpoint{
			{Node: "n12", Host: "203.0.113.12", DBPort: 9000, Eligible: true},
			{Node: "n13", Host: "203.0.113.13", DBPort: 9000, Eligible: true},
		},
	}}
	// When
	content, err := RenderConnectionInstructionsText(report)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"  - Server: 203.0.113.12", "  - Port: 9000",
		"Multi-host server list: 203.0.113.12,203.0.113.13", "Use port 9000",
	} {
		assertContains(t, content, value)
	}
	if strings.Contains(content, "  - Server: private-primary.example") {
		t.Fatalf("private primary must not be advertised for SQL clients: %s", content)
	}
}

func TestConnectionInstructionsReportNoPublicEndpoints(t *testing.T) {
	t.Parallel()
	// Given
	report := &DeploymentInfoReport{Connection: &ConnectionDetails{
		Hostname: "private-primary.example", DBPort: 8563,
		Endpoints: []config.ConnectionEndpoint{},
	}}
	// When
	content, err := RenderConnectionInstructionsText(report)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	assertContains(
		t,
		content,
		"No public database endpoints are recorded by the infrastructure preset.",
	)
	if strings.Contains(content, "  - Server:") {
		t.Fatalf("SQL clients must not be directed to the unexposed primary: %s", content)
	}
}
