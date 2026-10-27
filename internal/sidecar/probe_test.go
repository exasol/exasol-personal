// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar_test

import (
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const probedContainer = "image: caddy:2\n" +
	"      ports: [{name: web, containerPort: 8080, hostPort: 18080}, {containerPort: 9090}]\n"

func TestProbesParseWithKubernetesDefaults(t *testing.T) {
	t.Parallel()
	// Given
	data := containerDocument(probedContainer +
		"      readinessProbe: {httpGet: {port: web}, initialDelaySeconds: 5}")
	// When
	document, err := sidecar.Parse(data)
	require.NoError(t, err)
	container := document.Sidecars["example"].Container.Defaults()
	// Then
	require.Equal(t, &sidecar.Probe{
		HTTPGet: &sidecar.HTTPGetAction{
			Path: "/", Port: sidecar.PortReference{Name: "web"}, Scheme: sidecar.HTTP,
		},
		InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 1, FailureThreshold: 3,
	}, container.ReadinessProbe)
	require.Equal(t, 35, container.ReadinessProbe.Window())
}

func TestPortReferenceKeepsItsForm(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name      string
		reference sidecar.PortReference
		want      string
	}{
		{"number", sidecar.PortReference{Number: 8080}, "port: 8080\n"},
		{"name", sidecar.PortReference{Name: "web"}, "port: web\n"},
		{"quoted number", sidecar.PortReference{Name: "8080"}, "port: \"8080\"\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			action := sidecar.TCPSocketAction{Port: fixture.reference}
			// When
			data, err := yaml.Marshal(action)
			require.NoError(t, err)
			var parsed sidecar.TCPSocketAction
			err = yaml.Unmarshal(data, &parsed)
			// Then
			require.NoError(t, err)
			require.Equal(t, fixture.want, string(data))
			require.Equal(t, fixture.reference, parsed.Port)
		})
	}
}

func TestProbeValidationNamesTheOffendingField(t *testing.T) {
	t.Parallel()
	const readiness = "sidecars.example.container.readinessProbe"
	for _, fixture := range []struct{ name, probe, location string }{
		{"no handler", "readinessProbe: {periodSeconds: 5}", readiness},
		{
			"two handlers",
			"readinessProbe: {exec: {command: [\"true\"]}, tcpSocket: {port: web}}",
			readiness,
		},
		{
			"negative timing",
			"readinessProbe: {exec: {command: [\"true\"]}, failureThreshold: -1}",
			readiness + ".failureThreshold",
		},
		{"empty command", "readinessProbe: {exec: {command: []}}", readiness + ".exec.command"},
		{
			"relative path",
			"readinessProbe: {httpGet: {port: web, path: health}}",
			readiness + ".httpGet.path",
		},
		{
			"scheme",
			"readinessProbe: {httpGet: {port: web, scheme: FTP}}",
			readiness + ".httpGet.scheme",
		},
		{
			"undeclared port",
			"readinessProbe: {httpGet: {port: other}}",
			readiness + ".httpGet.port",
		},
		{
			"unpublished port",
			"readinessProbe: {tcpSocket: {port: 9090}}",
			readiness + ".tcpSocket.port",
		},
		{"unsupported field", "readinessProbe: {exec: {command: [\"true\"]}, grpc: {}}", "grpc"},
		{"liveness probe", "livenessProbe: {exec: {command: [\"true\"]}}", "livenessProbe"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			data := containerDocument(probedContainer + "      " + fixture.probe)
			// When
			_, err := sidecar.Parse(data)
			// Then
			require.ErrorContains(t, err, fixture.location)
		})
	}
}

func TestProbeDefaultsLeaveTheSourceUnchanged(t *testing.T) {
	t.Parallel()
	// Given
	container := sidecar.Container{
		Image:          "caddy:2",
		ReadinessProbe: &sidecar.Probe{Exec: &sidecar.ExecAction{Command: []string{"true"}}},
	}
	// When
	defaulted := container.Defaults()
	defaulted.ReadinessProbe.Exec.Command[0] = "false"
	// Then
	require.Zero(t, container.ReadinessProbe.PeriodSeconds)
	require.Equal(t, []string{"true"}, container.ReadinessProbe.Exec.Command)
}
