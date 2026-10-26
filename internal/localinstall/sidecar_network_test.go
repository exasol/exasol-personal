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

type networkEnvironment struct {
	sidecarEnvironment

	networks       []podmanNetwork
	database       string
	databaseAbsent bool
}

func (environment *networkEnvironment) Run(
	_ context.Context, _ io.Reader, stdout, _ io.Writer, command ...string,
) error {
	environment.calls = append(environment.calls, slices.Clone(command))
	if command[1] == "ps" {
		listing := `[{"Names":["exasol-db-test"]}]`
		if environment.databaseAbsent {
			listing = "[]"
		}
		_, err := io.WriteString(stdout, listing)

		return err
	}
	if command[1] == "container" {
		_, err := io.WriteString(stdout, environment.database)
		return err
	}
	switch command[2] {
	case "inspect", "ls":
		return json.NewEncoder(stdout).Encode(environment.networks)
	case "create", "connect", "rm":
		return nil
	default:
		return fmt.Errorf("unexpected command: %q", command)
	}
}

func newNetworkFixture(t *testing.T) (*SidecarRuntime, *networkEnvironment) {
	t.Helper()
	environment := &networkEnvironment{networks: []podmanNetwork{{
		Name: "exasol-db-test-services", Driver: "bridge", DNSEnabled: true,
		Labels: map[string]string{sidecarOwnerLabel: "exasol-db-test"},
	}}}
	runtime, err := NewSidecarRuntime(
		newPodmanRuntime(environment), "exasol-db-test", IdentityRuntimePath(t.TempDir()),
	)
	require.NoError(t, err)

	return runtime, environment
}

func TestSidecarNetworkValidatesOwnershipAndDNS(t *testing.T) {
	t.Parallel()
	for _, problem := range []string{"none", "foreign", "DNS disabled", "driver", "missing"} {
		t.Run(problem, func(t *testing.T) {
			t.Parallel()
			// Given
			runtime, environment := newNetworkFixture(t)
			switch problem {
			case "foreign":
				environment.networks[0].Labels = nil
			case "DNS disabled":
				environment.networks[0].DNSEnabled = false
			case "driver":
				environment.networks[0].Driver = "macvlan"
			case "missing":
				environment.networks = nil
			default:
			}
			// When
			err := runtime.EnsureNetwork(context.Background())
			// Then
			if (err != nil) != (problem != "none") {
				t.Fatalf("network validation = %v", err)
			}
			want := [][]string{
				{
					"podman", "network", "create", "--ignore", "--driver", "bridge", "--label",
					sidecarOwnerLabel + "=exasol-db-test", "exasol-db-test-services",
				},
				{"podman", "network", "inspect", "exasol-db-test-services"},
			}
			require.Equal(t, want, environment.calls)
		})
	}
}

func attachmentConnected(t *testing.T, calls [][]string) bool {
	t.Helper()
	connected := false
	for _, command := range calls {
		if slices.Contains(command, "stop") || slices.Contains(command, "restart") ||
			slices.Contains(command, "rm") {
			t.Fatalf("attachment changed database lifecycle: %q", command)
		}
		if command[1] != "network" || command[2] != "connect" {
			continue
		}
		connected = true
		require.Equal(t, []string{
			"podman", "network", "connect",
			"--alias", "database", "exasol-db-test-services", "exasol-db-test",
		}, command)
	}

	return connected
}

func TestSidecarDatabaseNetworkAttachment(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, mode, networks string
		restart, connect     bool
	}{
		{"pasta", "pasta", `{}`, true, false},
		{"slirp", "slirp4netns:allow_host_loopback=true", `{}`, true, false},
		{"host", "host", `{}`, true, false},
		{"bridge", "bridge", `{}`, false, true},
		{
			"attached", "bridge",
			`{"exasol-db-test-services":{"Aliases":["database"]}}`, false, false,
		},
		{
			"missing alias", "bridge",
			`{"exasol-db-test-services":{"Aliases":[]}}`, true, false,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			runtime, environment := newNetworkFixture(t)
			environment.database = fmt.Sprintf(
				`[{"HostConfig":{"NetworkMode":%q},"NetworkSettings":{"Networks":%s}}]`,
				fixture.mode, fixture.networks)
			// When
			restart, err := runtime.AttachDatabase(context.Background())
			// Then
			if err != nil || restart != fixture.restart {
				t.Fatalf("attach: restart=%t error=%v", restart, err)
			}
			require.Equal(t, fixture.connect, attachmentConnected(t, environment.calls))
			if fixture.mode != "bridge" && len(environment.calls) != 2 {
				t.Fatal("non-bridge detection mutated networking")
			}
		})
	}
}

func TestSidecarDatabaseQueriesTolerateStoppedDeployment(t *testing.T) {
	t.Parallel()
	// Given
	runtime, environment := newNetworkFixture(t)
	environment.databaseAbsent = true
	// When
	restart, attachErr := runtime.AttachDatabase(context.Background())
	required, inspectErr := runtime.DatabaseRestartRequired(context.Background())
	// Then
	require.NoError(t, attachErr)
	require.NoError(t, inspectErr)
	require.False(t, restart)
	require.False(t, required)
	for _, command := range environment.calls {
		require.Equal(t, "ps", command[1], "queried a database container that does not exist")
	}
}

func TestSidecarNetworkRemovalPreservesForeignResources(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"owned", "foreign", "absent"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			// Given
			runtime, environment := newNetworkFixture(t)
			switch state {
			case "foreign":
				environment.networks[0].Labels = nil
			case "absent":
				environment.networks = nil
			default:
			}
			// When
			err := runtime.RemoveNetwork(context.Background())
			// Then
			if (err != nil) != (state == "foreign") {
				t.Fatalf("removal = %v", err)
			}
			if state != "owned" {
				require.Len(t, environment.calls, 1, "removed absent or foreign network")

				return
			}
			require.Equal(t, [][]string{
				environment.calls[0],
				{"podman", "network", "rm", "exasol-db-test-services"},
			}, environment.calls)
		})
	}
}

func TestSidecarPortPublicationUsesExplicitLoopbackMappings(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name string
		port sidecar.Port
		want string
	}{
		{"internal", sidecar.Port{ContainerPort: 8080}, ""},
		{"default", sidecar.Port{ContainerPort: 8080, HostPort: 18080}, "127.0.0.1:18080:8080/tcp"},
		{
			"IPv6",
			sidecar.Port{ContainerPort: 8080, HostPort: 18080, HostIP: "::1"},
			"[::1]:18080:8080/tcp",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Given
			environment := &sidecarEnvironment{}
			runtime := newTestSidecarRuntime(t, environment)
			definition := sidecar.Container{
				Name: "example", Image: "caddy:2", Ports: []sidecar.Port{fixture.port},
			}
			// When
			err := runtime.Ensure(context.Background(), definition, nil)
			// Then
			require.NoError(t, err)
			args := environment.createCalls[0]
			index := slices.Index(args, "--publish")
			if fixture.want == "" {
				if index >= 0 || slices.Contains(args, "--publish-all") {
					t.Fatal("internal port was published")
				}
			} else if index < 0 || args[index+1] != fixture.want {
				t.Fatalf("publication = %s", strings.Join(args, " "))
			}
		})
	}
}
