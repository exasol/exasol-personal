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
	for _, multiNode := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "unlimited"}[multiNode], func(t *testing.T) {
			t.Parallel()
			// Given
			report := &DeploymentInfoReport{
				Connection: &ConnectionDetails{
					Hostname: "primary.example", DBPort: 8563,
					Endpoints: []config.ConnectionEndpoint{
						{Host: "203.0.113.11", DBPort: 8563, Eligible: true},
						{Host: "203.0.113.12", DBPort: 8563, Eligible: multiNode},
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
			) != multiNode {
				t.Fatalf("instructions advertise the wrong endpoints: %s", content)
			}
		})
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
