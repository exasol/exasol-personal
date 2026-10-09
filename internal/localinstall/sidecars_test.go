// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

type sidecarEnvironment struct {
	ExecutionEnvironment

	containers  []SidecarContainer
	calls       [][]string
	createCalls [][]string
	createEnv   map[string]string
	startError  error
	removeError error
}

func (environment *sidecarEnvironment) Run(
	_ context.Context,
	env map[string]string,
	_ io.Reader,
	stdout, stderr io.Writer,
	command ...string,
) error {
	if command[1] == "run" {
		environment.createCalls = append(environment.createCalls, slices.Clone(command))
		environment.createEnv = env
		if environment.startError != nil {
			_, _ = fmt.Fprint(stderr, strings.Join(command, " "))
		}

		return environment.startError
	}
	environment.calls = append(environment.calls, slices.Clone(command))
	if command[1] == "ps" {
		return json.NewEncoder(stdout).Encode(environment.containers)
	}
	if command[1] == "rm" {
		return environment.removeError
	}

	return fmt.Errorf("unexpected command: %q", command)
}

func newTestSidecarRuntime(t *testing.T, environment *sidecarEnvironment) *SidecarRuntime {
	t.Helper()
	runtime, err := NewSidecarRuntime(environment, "exasol-db-test")
	require.NoError(t, err)

	return runtime
}

func TestSidecarOptionalReferenceAbsent(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	definition := sidecar.Container{Name: "example", Image: "caddy:2", Env: []sidecar.EnvVar{{
		Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{SecretKeyRef: &sidecar.SecretKeyRef{
			Name: sidecar.DatabaseSource, Key: "password", Optional: true,
		}},
	}}}
	// When
	err := runtime.Ensure(context.Background(), definition, nil)
	// Then
	require.NoError(t, err)
	require.Len(t, environment.createCalls, 1)
	require.NotContains(t, environment.createCalls[0], "--env")
	require.Empty(t, environment.createEnv)
}

func TestSidecarSupportedExecutionSettings(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	marker := "hello world"
	definition := sidecar.Container{
		Name: "example", Image: "caddy:2", Command: []string{"caddy"},
		Args: []string{"respond", "--body", "$(MARKER)"}, WorkingDir: "/srv",
		ImagePullPolicy: sidecar.Never, RestartPolicy: sidecar.Always,
		Env: []sidecar.EnvVar{{Name: "MARKER", Value: &marker}},
	}
	fingerprint, err := SidecarDefinitionHash(definition.Defaults())
	require.NoError(t, err)
	// When
	err = runtime.Ensure(context.Background(), definition, nil)
	// Then
	require.NoError(t, err)
	want := []string{
		"podman",
		"run",
		"--detach",
		"--name",
		"exasol-db-test-sidecar-example",
		"--label",
		sidecarOwnerLabel + "=exasol-db-test",
		"--label",
		SidecarNameLabel + "=example",
		"--label",
		sidecarDefinitionLabel + "=" + fingerprint,
		"--pull",
		"never",
		"--restart",
		"always",
		"--network",
		"exasol-db-test-services",
		"--network-alias",
		"example",
		"--entrypoint",
		`["caddy"]`,
		"--workdir",
		"/srv",
		"--env",
		"MARKER",
		"caddy:2",
		"respond",
		"--body",
		"hello world",
	}
	if len(environment.createCalls) != 1 || !reflect.DeepEqual(environment.createCalls[0], want) {
		t.Fatalf("create calls = %q, want %q", environment.createCalls, want)
	}
	require.Equal(t, map[string]string{"MARKER": marker}, environment.createEnv)
}

func TestSidecarGuestPublicationPreservesDesiredDefinition(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	runtime.PublishPort = func(port sidecar.Port) string {
		return fmt.Sprintf("0.0.0.0::%d/tcp", port.ContainerPort)
	}
	definition := sidecar.Container{
		Name: "caddy", Image: "caddy:2",
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}, {ContainerPort: 9090}},
	}
	fingerprint, err := SidecarDefinitionHash(definition.Defaults())
	require.NoError(t, err)
	// When
	err = runtime.Ensure(t.Context(), definition, nil)
	// Then
	require.NoError(t, err)
	args := environment.createCalls[0]
	if args[slices.Index(args, "--publish")+1] != "0.0.0.0::8080/tcp" ||
		!slices.Contains(args, sidecarDefinitionLabel+"="+fingerprint) ||
		strings.Count(strings.Join(args, " "), "--publish") != 1 {
		t.Fatalf("guest publication = %q", args)
	}
	require.Equal(t, 18080, definition.Ports[0].HostPort)
}

func TestSidecarImageAndPolicyDefaults(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ policy, want string }{
		{"", "on-failure"},
		{sidecar.OnFailure, "on-failure"},
		{sidecar.Always, "always"},
		{sidecar.Never, "no"},
	} {
		t.Run(fixture.policy, func(t *testing.T) {
			t.Parallel()
			// Given
			environment := &sidecarEnvironment{}
			runtime := newTestSidecarRuntime(t, environment)
			definition := sidecar.Container{
				Name:          "example",
				Image:         "caddy:2",
				RestartPolicy: fixture.policy,
			}
			// When
			err := runtime.Ensure(context.Background(), definition, nil)
			// Then
			require.NoError(t, err)
			args := environment.createCalls[0]
			if slices.Contains(args, "--entrypoint") || args[len(args)-1] != "caddy:2" ||
				args[slices.Index(args, "--restart")+1] != fixture.want ||
				args[slices.Index(args, "--pull")+1] != "missing" {
				t.Fatalf("wrong defaults: %q", args)
			}
		})
	}
}

func TestSidecarReconciliationKeepsOrReplacesOwnedContainer(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, state, fingerprint string
		recreate                 bool
	}{
		{"unchanged", "running", "same", false},
		{"edited", "running", "old", true},
		{"exited", "exited", "same", true},
		{"partial", "created", "same", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			definition := sidecar.Container{Name: "example", Image: "caddy:2"}
			hash, err := SidecarDefinitionHash(definition.Defaults())
			require.NoError(t, err)
			if fixture.fingerprint == "old" {
				hash = "old"
			}
			environment := &sidecarEnvironment{containers: []SidecarContainer{{
				ID: "owned-id", State: fixture.state,
				Labels: map[string]string{
					sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "example",
					sidecarDefinitionLabel: hash,
				},
			}}}
			runtime := newTestSidecarRuntime(t, environment)
			// When
			err = runtime.Ensure(context.Background(), definition, nil)
			// Then
			require.NoError(t, err)
			if (len(environment.createCalls) == 1) != fixture.recreate {
				t.Fatal("wrong recreation decision")
			}
			if fixture.recreate && !reflect.DeepEqual(environment.calls[1],
				[]string{"podman", "rm", "--force", "--ignore", "--", "owned-id"}) {
				t.Fatal("wrong cleanup target")
			}
		})
	}
}

func TestSidecarCredentialProtection(t *testing.T) {
	t.Parallel()
	// Given
	password := "disposable-test-password"
	environment := &sidecarEnvironment{startError: errors.New(password + " bind failed")}
	runtime := newTestSidecarRuntime(t, environment)
	definition := sidecar.Container{
		Name: "example", Image: "caddy:2",
		Env: []sidecar.EnvVar{{Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{
			SecretKeyRef: &sidecar.SecretKeyRef{Name: sidecar.DatabaseSource, Key: "password"},
		}}},
	}
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, config.WriteSidecars(deployment, sidecar.Document{
		Version: 1, Containers: []sidecar.Container{definition},
	}))
	// When
	err := runtime.Ensure(context.Background(), definition, sidecar.Sources{
		sidecar.DatabaseSource: {"password": password},
	})
	// Then
	if err == nil || strings.Contains(err.Error(), password) ||
		!strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("unexpected diagnostic: %v", err)
	}
	for _, command := range environment.calls {
		if strings.Contains(strings.Join(command, " "), password) {
			t.Fatal("credential in public command input")
		}
	}
	require.Len(t, environment.createCalls, 1)
	require.NotContains(t, strings.Join(environment.createCalls[0], " "), password)
	require.Equal(t, password, environment.createEnv["PASSWORD"])
	info, statErr := os.Stat(deployment.SidecarsPath())
	require.NoError(t, statErr)
	if goruntime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestSidecarCreationRetriesAfterRuntimeFailure(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{startError: errors.New("registry unavailable")}
	runtime := newTestSidecarRuntime(t, environment)
	definition := sidecar.Container{Name: "example", Image: "caddy:2"}
	// When
	first := runtime.Ensure(context.Background(), definition, nil)
	environment.startError = nil
	second := runtime.Ensure(context.Background(), definition, nil)
	// Then
	if first == nil || second != nil || len(environment.createCalls) != 2 {
		t.Fatalf(
			"creation retries: first=%v, second=%v, calls=%q",
			first,
			second,
			environment.createCalls,
		)
	}
}

func TestSidecarCleanupSelectsOnlyRemovedDefinitions(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"remove", "absent"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given
			environment := &sidecarEnvironment{containers: []SidecarContainer{
				{ID: "keep-id", Labels: map[string]string{
					sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "keep",
				}},
				{ID: "remove-id", Labels: map[string]string{
					sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "remove",
				}},
				{ID: "foreign", Labels: map[string]string{
					sidecarOwnerLabel: "other", SidecarNameLabel: name,
				}},
			}}
			runtime := newTestSidecarRuntime(t, environment)
			// When
			err := runtime.Remove(context.Background(), name)
			// Then
			require.NoError(t, err)
			if name == "absent" {
				require.Len(t, environment.calls, 1)
			} else if len(environment.calls) != 2 || !reflect.DeepEqual(environment.calls[1],
				[]string{"podman", "rm", "--force", "--ignore", "--", "remove-id"}) {
				t.Fatalf("unexpected cleanup: %q", environment.calls)
			}
		})
	}
}
