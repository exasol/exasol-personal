// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type sidecarEnvironment struct {
	ExecutionEnvironment

	containers      []podmanListing
	calls           [][]string
	createCalls     [][]string
	createEnv       map[string]string
	createEnvMode   os.FileMode
	startError      error
	startDiagnostic string
	removeError     error
	// The archive copied into the container, by path below its root.
	copied     map[string]string
	copiedMode map[string]os.FileMode
}

func (environment *sidecarEnvironment) Run(
	_ context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
	command ...string,
) error {
	if command[1] == "run" || command[1] == "create" {
		environment.createCalls = append(environment.createCalls, slices.Clone(command))
		environment.createEnv, environment.createEnvMode = readEnvFileArgument(command)
		if environment.startError != nil {
			diagnostic := environment.startDiagnostic
			if diagnostic == "" {
				diagnostic = strings.Join(command, " ")
			}
			_, _ = fmt.Fprint(stderr, diagnostic)
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
	if command[1] == "cp" {
		return environment.readArchive(stdin)
	}
	if command[1] == "start" {
		return nil
	}

	return fmt.Errorf("unexpected command: %q", command)
}

// readEnvFileArgument reads the file podman is being pointed at, before the
// deferred removal takes it away.
func readEnvFileArgument(command []string) (map[string]string, os.FileMode) {
	index := slices.Index(command, "--env-file")
	if index < 0 || index+1 >= len(command) {
		return nil, 0
	}
	info, err := os.Stat(command[index+1])
	if err != nil {
		return nil, 0
	}
	content, err := os.ReadFile(command[index+1])
	if err != nil {
		return nil, 0
	}
	values := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(content)), "\n") {
		if name, value, found := strings.Cut(line, "="); found {
			values[name] = value
		}
	}

	return values, info.Mode().Perm()
}

func (environment *sidecarEnvironment) readArchive(stdin io.Reader) error {
	environment.copied = map[string]string{}
	environment.copiedMode = map[string]os.FileMode{}
	reader := tar.NewReader(stdin)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		environment.copied[header.Name] = string(content)
		environment.copiedMode[header.Name] = header.FileInfo().Mode().Perm()
	}
}

func definitionFor(container sidecar.Container) sidecar.Definition {
	return sidecar.Definition{Container: container}
}

func newTestSidecarRuntime(t *testing.T, environment *sidecarEnvironment) *SidecarRuntime {
	t.Helper()
	runtime, err := NewSidecarRuntime(
		newPodmanRuntime(environment), "exasol-db-test", IdentityRuntimePath(t.TempDir()),
	)
	require.NoError(t, err)

	return runtime
}

func testFingerprint(
	t *testing.T, runtime *SidecarRuntime, definition sidecar.Container,
) string {
	t.Helper()
	resolved, err := sidecar.Resolve(definition, nil)
	require.NoError(t, err)
	fingerprint, err := runtime.fingerprint(resolved, nil)
	require.NoError(t, err)

	return fingerprint
}

func TestSidecarStartupFailureCarriesCauseAndDiagnostic(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{
		startError:      errors.New("exit status 126"),
		startDiagnostic: "Error: rootlessport 127.0.0.1:18080: bind: address already in use",
	}
	runtime := newTestSidecarRuntime(t, environment)
	definition := sidecar.Container{
		Name: "example", Image: "caddy:2",
		Ports: []sidecar.Port{{ContainerPort: 8080, HostPort: 18080}},
	}
	// When
	err := runtime.Ensure(t.Context(), definitionFor(definition), nil)
	// Then
	startup, ok := errors.AsType[*sidecar.StartupError](err)
	require.True(t, ok)
	require.Equal(t, "example", startup.Sidecar)
	require.Contains(t, startup.Cause, "18080")
	require.Equal(t, 1, strings.Count(startup.Cause, "address already in use"))
	require.Contains(t, startup.Diagnostic, "address already in use")
	require.Contains(t, err.Error(), "sidecar example")
}

func TestSidecarOptionalReferenceAbsent(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	definition := sidecar.Container{Name: "example", Image: "caddy:2", Env: []sidecar.EnvVar{{
		Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{SecretKeyRef: &sidecar.SecretKeyRef{
			Name: sidecar.DatabaseSecret, Key: "password", Optional: true,
		}},
	}}}
	// When
	err := runtime.Ensure(context.Background(), definitionFor(definition), nil)
	// Then
	require.NoError(t, err)
	require.Len(t, environment.createCalls, 1)
	require.NotContains(t, environment.createCalls[0], "--env-file")
	require.Empty(t, environment.createEnv)
}

func TestSidecarRejectsUnrepresentableValue(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, password string }{
		{"line feed", "disposable\nsecret"},
		{"carriage return", "disposable\rsecret"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			environment := &sidecarEnvironment{}
			runtime := newTestSidecarRuntime(t, environment)
			definition := sidecar.Container{
				Name: "example", Image: "caddy:2",
				Env: []sidecar.EnvVar{{Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{
					SecretKeyRef: &sidecar.SecretKeyRef{
						Name: sidecar.DatabaseSecret, Key: "password",
					},
				}}},
			}
			// When
			err := runtime.Ensure(
				context.Background(),
				definitionFor(definition),
				sidecar.Secrets{
					sidecar.DatabaseSecret: {
						"password": {Value: fixture.password, Sensitive: true},
					},
				},
			)
			// Then
			require.ErrorContains(t, err, "PASSWORD")
			require.ErrorContains(t, err, "line break")
			require.NotContains(t, err.Error(), fixture.password)
			require.Empty(t, environment.createCalls)
		})
	}
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
	fingerprint := testFingerprint(t, runtime, definition)
	// When
	err := runtime.Ensure(context.Background(), definitionFor(definition), nil)
	// Then
	require.NoError(t, err)
	want := []string{
		"podman",
		"run",
		"--detach",
		"--name",
		"exasol-db-test-sidecar-example",
		"--label",
		sidecarDefinitionLabel + "=" + fingerprint,
		"--label",
		SidecarNameLabel + "=example",
		"--label",
		sidecarOwnerLabel + "=exasol-db-test",
		"--pull",
		"never",
		"--restart",
		"always",
		"--network",
		"exasol-db-test-services",
		"--network-alias",
		"example",
		"--env-file",
		envFilePath(t, environment.createCalls[0]),
		"--entrypoint",
		`["caddy"]`,
		"--workdir",
		"/srv",
		"caddy:2",
		"respond",
		"--body",
		"hello world",
	}
	if len(environment.createCalls) != 1 || !reflect.DeepEqual(environment.createCalls[0], want) {
		t.Fatalf("create calls = %q, want %q", environment.createCalls, want)
	}
	require.Equal(t, map[string]string{"MARKER": marker}, environment.createEnv)
	require.Equal(t, os.FileMode(0o600), environment.createEnvMode)
	require.NoFileExists(t, envFilePath(t, environment.createCalls[0]))
}

func envFilePath(t *testing.T, command []string) string {
	t.Helper()
	index := slices.Index(command, "--env-file")
	require.GreaterOrEqual(t, index, 0)

	return command[index+1]
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
	fingerprint := testFingerprint(t, runtime, definition)
	// When
	err := runtime.Ensure(t.Context(), definitionFor(definition), nil)
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
		{"", "always"},
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
			err := runtime.Ensure(
				context.Background(),
				definitionFor(definition),
				nil,
			)
			// Then
			require.NoError(t, err)
			args := environment.createCalls[0]
			if args[slices.Index(args, "--restart")+1] != fixture.want ||
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
			hash := testFingerprint(
				t, newTestSidecarRuntime(t, &sidecarEnvironment{}), definition,
			)
			if fixture.fingerprint == "old" {
				hash = "old"
			}
			environment := &sidecarEnvironment{containers: []podmanListing{{
				ID: "owned-id", State: fixture.state,
				Labels: map[string]string{
					sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "example",
					sidecarDefinitionLabel: hash,
				},
			}}}
			runtime := newTestSidecarRuntime(t, environment)
			// When
			err := runtime.Ensure(
				context.Background(),
				definitionFor(definition),
				nil,
			)
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

func TestSidecarProbeEditKeepsTheFingerprint(t *testing.T) {
	t.Parallel()
	// Given
	runtime := newTestSidecarRuntime(t, &sidecarEnvironment{})
	definition := sidecar.Container{Name: "example", Image: "caddy:2"}
	probed := definition
	probed.ReadinessProbe = &sidecar.Probe{
		Exec: &sidecar.ExecAction{Command: []string{"true"}}, PeriodSeconds: 2,
	}
	// When
	original := testFingerprint(t, runtime, definition)
	edited := testFingerprint(t, runtime, probed)
	// Then
	require.Equal(t, original, edited)
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
			SecretKeyRef: &sidecar.SecretKeyRef{Name: sidecar.DatabaseSecret, Key: "password"},
		}}},
	}
	deployment := config.NewDeploymentDir(t.TempDir())
	require.NoError(t, config.WriteSidecars(deployment, sidecar.Document{
		Version: 1, Sidecars: map[string]sidecar.Definition{
			definition.Name: {Container: definition},
		},
	}))
	// When
	err := runtime.Ensure(
		context.Background(),
		definitionFor(definition),
		sidecar.Secrets{
			sidecar.DatabaseSecret: {
				"password": {Value: password, Sensitive: true},
			},
		},
	)
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
	first := runtime.Ensure(context.Background(), definitionFor(definition), nil)
	environment.startError = nil
	second := runtime.Ensure(context.Background(), definitionFor(definition), nil)
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
			environment := &sidecarEnvironment{containers: []podmanListing{
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

func passwordVolume() []sidecar.Volume {
	return []sidecar.Volume{{Name: "database", Secret: &sidecar.SecretVolume{
		SecretName: sidecar.DatabaseSecret,
		Items:      []sidecar.KeyToPath{{Key: sidecar.DatabasePasswordKey, Path: "password"}},
	}}}
}

func fileDefinition() sidecar.Definition {
	file := "/run/exasol-database/password"

	return sidecar.Definition{
		Container: sidecar.Container{
			Name: "example", Image: "example:1",
			Env: []sidecar.EnvVar{{Name: "PASSWORD_FILE", Value: &file}},
			VolumeMounts: []sidecar.VolumeMount{
				{Name: "database", MountPath: "/run/exasol-database", ReadOnly: true},
			},
		},
		Volumes: passwordVolume(),
	}
}

func passwordSecrets(password string) sidecar.Secrets {
	return sidecar.Secrets{sidecar.DatabaseSecret: {
		sidecar.DatabasePasswordKey: {Value: password, Sensitive: true},
	}}
}

func TestSidecarCopiesVolumeFilesIntoTheContainer(t *testing.T) {
	t.Parallel()
	// Given
	password := "disposable-test-password"
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	// When
	err := runtime.Ensure(t.Context(), fileDefinition(), passwordSecrets(password))
	// Then
	require.NoError(t, err)
	require.Len(t, environment.createCalls, 1)
	args := environment.createCalls[0]
	require.Equal(t, "create", args[1])
	require.NotContains(t, strings.Join(args, " "), password)
	fileVariable := map[string]string{ //nolint:gosec // A path, not a credential.
		"PASSWORD_FILE": "/run/exasol-database/password",
	}
	require.Equal(t, fileVariable, environment.createEnv)
	require.Equal(t, map[string]string{
		"run/exasol-database/":         "",
		"run/exasol-database/password": password,
	}, environment.copied)
	require.Equal(t, map[string]os.FileMode{
		"run/exasol-database/":         0o555,
		"run/exasol-database/password": 0o444,
	}, environment.copiedMode)
	name := "exasol-db-test-sidecar-example"
	require.Equal(t, [][]string{
		{"podman", "cp", "-", name + ":/"},
		{"podman", "start", name},
	}, environment.calls[len(environment.calls)-2:])
	require.Equal(t, "example:1", args[len(args)-1])
}

func TestSidecarVolumeWithoutItemsProjectsEveryKey(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	definition := fileDefinition()
	definition.Volumes[0].Secret.Items = nil
	secrets := passwordSecrets("secret")
	secrets[sidecar.DatabaseSecret]["host"] = sidecar.SecretValue{Value: "database"}
	// When
	err := runtime.Ensure(t.Context(), definition, secrets)
	// Then
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"run/exasol-database/":         "",
		"run/exasol-database/host":     "database",
		"run/exasol-database/password": "secret",
	}, environment.copied)
}

func TestSidecarUnavailableVolumeKey(t *testing.T) {
	t.Parallel()
	for _, optional := range []bool{false, true} {
		t.Run(fmt.Sprint("optional=", optional), func(t *testing.T) {
			t.Parallel()
			// Given
			environment := &sidecarEnvironment{}
			runtime := newTestSidecarRuntime(t, environment)
			definition := fileDefinition()
			definition.Volumes[0].Secret.Optional = optional
			// When
			err := runtime.Ensure(t.Context(), definition, sidecar.Secrets{
				sidecar.DatabaseSecret: {"host": {Value: "database"}},
			})
			// Then
			if optional {
				require.NoError(t, err)
				require.Equal(t, "run", environment.createCalls[0][1])
				require.Nil(t, environment.copied)
			} else {
				require.ErrorContains(t, err, "required secret exasol-database key password")
				require.Empty(t, environment.createCalls)
			}
		})
	}
}

func TestSidecarVolumeDiagnosticsWithholdThePassword(t *testing.T) {
	t.Parallel()
	// Given
	password := "disposable-test-password"
	environment := &sidecarEnvironment{startError: errors.New(password + " bind failed")}
	runtime := newTestSidecarRuntime(t, environment)
	// When
	err := runtime.Ensure(t.Context(), fileDefinition(), passwordSecrets(password))
	// Then
	require.ErrorContains(t, err, "bind failed")
	require.NotContains(t, err.Error(), password)
}

func TestSidecarFailedStartRecordsNothing(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{startError: errors.New("image unavailable")}
	runtime := newTestSidecarRuntime(t, environment)
	// When
	err := runtime.Ensure(t.Context(), fileDefinition(), passwordSecrets("secret"))
	// Then
	require.Error(t, err)
	require.NoFileExists(t, runtime.recordPath("example"))
}

func TestSidecarReplacementDropsThePreviousRecord(t *testing.T) {
	t.Parallel()
	// Given
	definition := fileDefinition()
	environment := &sidecarEnvironment{startError: errors.New("interrupted")}
	runtime := newTestSidecarRuntime(t, environment)
	previous, files, err := resolveDefinition(definition, passwordSecrets("previous-password"))
	require.NoError(t, err)
	require.NoError(t, runtime.recordApplied("example", previous, files))
	fingerprint, err := runtime.fingerprint(previous, definition.Volumes)
	require.NoError(t, err)
	environment.containers = []podmanListing{{
		ID: "owned-id", State: "running",
		Labels: map[string]string{
			sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "example",
			sidecarDefinitionLabel: fingerprint,
		},
	}}
	// When
	err = runtime.Ensure(t.Context(), definition, passwordSecrets("current-password"))
	// Then
	require.Error(t, err)
	current, err := runtime.appliedCurrent("example", previous, files)
	require.NoError(t, err)
	require.False(t, current)
}

func TestSidecarLabelOmitsSensitiveValues(t *testing.T) {
	t.Parallel()
	for name, definition := range map[string]sidecar.Definition{
		"environment": {Container: sidecar.Container{
			Name: "example", Image: "example:1",
			Env: []sidecar.EnvVar{{Name: "PASSWORD", ValueFrom: &sidecar.EnvSource{
				SecretKeyRef: &sidecar.SecretKeyRef{
					Name: sidecar.DatabaseSecret, Key: sidecar.DatabasePasswordKey,
				},
			}}},
		}},
		"volume": fileDefinition(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given
			runtime := newTestSidecarRuntime(t, &sidecarEnvironment{})
			label := func(password string) string {
				resolved, files, err := resolveDefinition(definition, passwordSecrets(password))
				require.NoError(t, err)
				require.NotEmpty(t, append(files, sidecar.File{}))
				fingerprint, err := runtime.fingerprint(resolved, definition.Volumes)
				require.NoError(t, err)

				return fingerprint
			}
			// When
			first, second := label("first-password"), label("second-password")
			// Then
			require.Equal(t, first, second)
		})
	}
}

func TestSidecarLabelKeepsDefinitionsWithoutVolumes(t *testing.T) {
	t.Parallel()
	// Given
	runtime := newTestSidecarRuntime(t, &sidecarEnvironment{})
	container := sidecar.Container{Name: "example", Image: "caddy:2"}
	resolved, err := sidecar.Resolve(container, nil)
	require.NoError(t, err)
	// When
	withNone, err := runtime.fingerprint(resolved, nil)
	require.NoError(t, err)
	withVolumes, err := runtime.fingerprint(resolved, passwordVolume())
	require.NoError(t, err)
	// Then
	identity, err := yaml.Marshal(container.Defaults())
	require.NoError(t, err)
	unchanged := sha256.Sum256(identity)
	require.Equal(t, hex.EncodeToString(unchanged[:]), withNone)
	require.NotEqual(t, withNone, withVolumes)
}

func TestSidecarRotatedSecretReplacesContainer(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, recorded string
		recreate       bool
	}{
		{"current", "current-password", false},
		{"rotated", "previous-password", true},
		{"unrecorded", "", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			definition := fileDefinition()
			secrets := passwordSecrets("current-password")
			environment := &sidecarEnvironment{}
			runtime := newTestSidecarRuntime(t, environment)
			if fixture.recorded != "" {
				resolved, files, err := resolveDefinition(
					definition, passwordSecrets(fixture.recorded),
				)
				require.NoError(t, err)
				require.NoError(t, runtime.recordApplied("example", resolved, files))
			}
			resolved, _, err := resolveDefinition(definition, secrets)
			require.NoError(t, err)
			fingerprint, err := runtime.fingerprint(resolved, definition.Volumes)
			require.NoError(t, err)
			environment.containers = []podmanListing{{
				ID: "owned-id", State: "running",
				Labels: map[string]string{
					sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "example",
					sidecarDefinitionLabel: fingerprint,
				},
			}}
			listed, err := runtime.List(t.Context())
			require.NoError(t, err)
			// When
			matches, matchErr := runtime.Matches(listed[0], definition, secrets)
			err = runtime.Ensure(t.Context(), definition, secrets)
			// Then
			require.NoError(t, matchErr)
			require.NoError(t, err)
			require.Equal(t, !fixture.recreate, matches)
			require.Equal(t, fixture.recreate, len(environment.createCalls) == 1)
			if fixture.recreate {
				require.Equal(t, "current-password",
					environment.copied["run/exasol-database/password"])
			}
		})
	}
}

func TestSidecarRemovalDeletesAppliedRecord(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{containers: []podmanListing{{
		ID: "owned-id", Labels: map[string]string{
			sidecarOwnerLabel: "exasol-db-test", SidecarNameLabel: "example",
		},
	}}}
	runtime := newTestSidecarRuntime(t, environment)
	resolved, files, err := resolveDefinition(fileDefinition(), passwordSecrets("secret"))
	require.NoError(t, err)
	require.NoError(t, runtime.recordApplied("example", resolved, files))
	// When
	err = runtime.Remove(t.Context(), "example")
	// Then
	require.NoError(t, err)
	require.NoFileExists(t, runtime.recordPath("example"))
}

func TestSidecarDroppedVolumeDeletesAppliedRecord(t *testing.T) {
	t.Parallel()
	// Given
	environment := &sidecarEnvironment{}
	runtime := newTestSidecarRuntime(t, environment)
	resolved, files, err := resolveDefinition(fileDefinition(), passwordSecrets("secret"))
	require.NoError(t, err)
	require.NoError(t, runtime.recordApplied("example", resolved, files))
	// When
	err = runtime.Ensure(t.Context(), sidecar.Definition{
		Container: sidecar.Container{Name: "example", Image: "caddy:2"},
	}, nil)
	// Then
	require.NoError(t, err)
	require.NoFileExists(t, runtime.recordPath("example"))
	require.Equal(t, "run", environment.createCalls[0][1])
	require.Nil(t, environment.copied)
}

func TestSidecarPruneRecordsKeepsDesiredSidecars(t *testing.T) {
	t.Parallel()
	// Given
	runtime := newTestSidecarRuntime(t, &sidecarEnvironment{})
	resolved, files, err := resolveDefinition(fileDefinition(), passwordSecrets("secret"))
	require.NoError(t, err)
	for _, name := range []string{"kept", "removed"} {
		require.NoError(t, runtime.recordApplied(name, resolved, files))
	}
	other := filepath.Join(runtime.EnvDir.HostPath, "kept.env")
	require.NoError(t, os.WriteFile(other, nil, 0o600))
	// When
	err = runtime.PruneRecords([]string{"kept"})
	// Then
	require.NoError(t, err)
	require.FileExists(t, runtime.recordPath("kept"))
	require.NoFileExists(t, runtime.recordPath("removed"))
	require.FileExists(t, other)
}
