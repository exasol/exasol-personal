// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

type systemdEnvironment struct {
	ExecutionEnvironment

	containers []SidecarContainer
	secrets    []string
	calls      [][]string
	units      map[string]string
	scripts    []string
	stdin      map[string]string
}

func (environment *systemdEnvironment) Run(
	_ context.Context,
	_ map[string]string,
	stdin io.Reader,
	stdout, _ io.Writer,
	command ...string,
) error {
	environment.calls = append(environment.calls, slices.Clone(command))
	input := ""
	if stdin != nil {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		input = string(content)
	}
	switch {
	case command[0] == "sh":
		environment.scripts = append(environment.scripts, command[2])
		if strings.Contains(command[2], "enable --now") {
			environment.units[command[4]] = input
		}
		if strings.Contains(command[2], "disable --now") {
			delete(environment.units, command[4])
		}

		return nil
	case command[1] == "ps":
		return json.NewEncoder(stdout).Encode(environment.containers)
	case command[1] == "secret" && command[2] == "ls":
		_, err := io.WriteString(stdout, strings.Join(environment.secrets, "\n"))

		return err
	case command[1] == "secret" && command[2] == "create":
		environment.secrets = append(environment.secrets, command[4])
		environment.stdin[command[4]] = input

		return nil
	case command[1] == "secret" && command[2] == "rm":
		environment.secrets = slices.DeleteFunc(environment.secrets, func(name string) bool {
			return name == command[4]
		})

		return nil
	case command[1] == "rm" || command[1] == "run":
		return nil
	}

	return fmt.Errorf("unexpected command: %q", command)
}

func newTestSystemdRuntime(
	t *testing.T,
	environment *systemdEnvironment,
) *SystemdSidecarRuntime {
	t.Helper()
	environment.units = map[string]string{}
	environment.stdin = map[string]string{}
	runtime, err := NewSystemdSidecarRuntime(
		environment, "exasol-db-test", "c4_cloud_command.service",
	)
	require.NoError(t, err)
	runtime.DatabaseHost = "10.0.1.11"

	return runtime
}

func cloudSidecarDefinition() sidecar.Container {
	return sidecar.Container{
		Name: "example", Image: "caddy:2-alpine", RestartPolicy: sidecar.OnFailure,
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 8080}},
		Env: []sidecar.EnvVar{{
			Name: "EXASOL_DB_PASSWORD",
			ValueFrom: &sidecar.EnvSource{SecretKeyRef: &sidecar.SecretKeyRef{
				Name: sidecar.DatabaseSource, Key: "password",
			}},
		}},
	}
}

func TestSystemdSidecarBindsUnitToDeploymentService(t *testing.T) {
	t.Parallel()
	// Given
	environment := &systemdEnvironment{}
	runtime := newTestSystemdRuntime(t, environment)
	sources := sidecar.Sources{sidecar.DatabaseSource: {"password": "secret-value"}}
	// When
	err := runtime.Ensure(t.Context(), cloudSidecarDefinition(), sources)
	// Then
	require.NoError(t, err)
	unit := environment.units["exasol-db-test-sidecar-example.service"]
	require.Contains(t, unit, "PartOf=c4_cloud_command.service")
	require.Contains(t, unit, "After=c4_cloud_command.service")
	require.Contains(t, unit, "WantedBy=c4_cloud_command.service")
	require.Contains(t, unit, "Restart=on-failure")
	require.Contains(t, unit, `ExecStart="/usr/bin/podman" "run" "--replace"`)
	require.Contains(t, unit, `"--restart" "no"`)
	require.Contains(t, unit, "ExecStop=/usr/bin/podman stop --ignore --time 10 "+
		"exasol-db-test-sidecar-example")
	require.NotContains(t, unit, "--detach")
	for _, call := range environment.calls {
		require.False(t, call[0] == podmanCommand && call[1] == "run",
			"the deployment service starts the container, not the launcher")
	}
}

func TestSystemdSidecarDeliversValuesAsContainerSecrets(t *testing.T) {
	t.Parallel()
	// Given
	environment := &systemdEnvironment{}
	runtime := newTestSystemdRuntime(t, environment)
	sources := sidecar.Sources{sidecar.DatabaseSource: {"password": "secret-value"}}
	// When
	err := runtime.Ensure(t.Context(), cloudSidecarDefinition(), sources)
	// Then
	require.NoError(t, err)
	secret := runtime.secretName("example", 0)
	require.Equal(t, []string{secret}, environment.secrets)
	require.Equal(t, "secret-value", environment.stdin[secret])
	unit := environment.units["exasol-db-test-sidecar-example.service"]
	require.Contains(t, unit, `"--secret" "`+secret+`,type=env,target=EXASOL_DB_PASSWORD"`)
	require.NotContains(t, unit, "secret-value")
	for _, call := range environment.calls {
		require.NotContains(t, call, "secret-value")
	}
}

func TestSystemdSidecarRemoveClearsUnitAndSecrets(t *testing.T) {
	t.Parallel()
	// Given
	environment := &systemdEnvironment{}
	runtime := newTestSystemdRuntime(t, environment)
	sources := sidecar.Sources{sidecar.DatabaseSource: {"password": "secret-value"}}
	require.NoError(t, runtime.Ensure(t.Context(), cloudSidecarDefinition(), sources))
	// When
	err := runtime.Remove(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.Empty(t, environment.units)
	require.Empty(t, environment.secrets)
	require.Contains(t, environment.scripts[len(environment.scripts)-1], "disable --now")
}

func TestSystemdSidecarReusesConvergedService(t *testing.T) {
	t.Parallel()
	// Given
	environment := &systemdEnvironment{}
	runtime := newTestSystemdRuntime(t, environment)
	sources := sidecar.Sources{sidecar.DatabaseSource: {"password": "secret-value"}}
	definition := cloudSidecarDefinition()
	require.NoError(t, runtime.Ensure(t.Context(), definition, sources))
	fingerprint, err := SidecarDefinitionHash(definition)
	require.NoError(t, err)
	environment.containers = []SidecarContainer{{
		ID: "one", State: "running", Labels: map[string]string{
			sidecarOwnerLabel:      "exasol-db-test",
			SidecarNameLabel:       "example",
			sidecarDefinitionLabel: fingerprint,
		},
	}}
	units, scripts := len(environment.units), len(environment.scripts)
	// When
	err = runtime.Ensure(t.Context(), definition, sources)
	// Then
	require.NoError(t, err)
	require.Len(t, environment.units, units)
	require.Len(t, environment.scripts, scripts)
}
