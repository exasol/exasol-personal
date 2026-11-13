// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/deploy"
	"github.com/stretchr/testify/require"
)

func TestInfoReportsEndpointEligibilityAndPreservesSingularFields(t *testing.T) {
	t.Parallel()
	// Given
	endpoints := []config.ConnectionEndpoint{
		{Node: "n11", Host: "203.0.113.11", DBPort: 8563, Eligible: true},
		{Node: "n12", Host: "203.0.113.12", DBPort: 8563, Eligible: false},
	}
	report := &deploy.DeploymentInfoReport{
		DeploymentState: deploy.StatusRunning,
		Connection: &deploy.ConnectionDetails{
			Hostname: "primary.example", DBPort: 8563, Endpoints: endpoints,
		},
	}
	// When
	var text, encoded bytes.Buffer
	require.NoError(t, renderDeploymentInfoText(&text, report))
	require.NoError(t, renderDeploymentInfoJSON(&encoded, report))
	// Then
	require.Contains(t, text.String(), "n11: 203.0.113.11:8563 (eligible)")
	require.Contains(t, text.String(), "n12: 203.0.113.12:8563 (topology only)")
	var actual deploy.DeploymentInfoReport
	require.NoError(t, json.Unmarshal(encoded.Bytes(), &actual))
	require.Equal(t, endpoints, actual.Connection.Endpoints)
	require.Equal(t, "primary.example", actual.Connection.Hostname)
	require.Equal(t, 8563, actual.Connection.DBPort)
}
