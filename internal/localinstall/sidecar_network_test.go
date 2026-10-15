// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/stretchr/testify/require"
)

type networkEnvironment struct {
	sidecarEnvironment

	networks []serviceNetwork
	database string
}

func (environment *networkEnvironment) Run(
	_ context.Context, _ map[string]string, _ io.Reader, stdout, _ io.Writer, command ...string,
) error {
	environment.calls = append(environment.calls, slices.Clone(command))
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
	environment := &networkEnvironment{networks: []serviceNetwork{{
		Name: "exasol-db-test-services", Driver: "bridge", DNSEnabled: true,
		Labels: map[string]string{sidecarOwnerLabel: "exasol-db-test"},
	}}}
	runtime, err := NewSidecarRuntime(environment, "exasol-db-test")
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
			connected := false
			for _, command := range environment.calls {
				if command[1] == "network" && command[2] == "connect" {
					connected = true
					if !reflect.DeepEqual(command, []string{
						"podman", "network", "connect",
						"--alias", "database", "exasol-db-test-services", "exasol-db-test",
					}) {
						t.Fatalf("attachment = %q", command)
					}
				}
				if slices.Contains(command, "stop") || slices.Contains(command, "restart") ||
					slices.Contains(command, "rm") {
					t.Fatalf("attachment changed database lifecycle: %q", command)
				}
			}
			require.Equal(t, fixture.connect, connected)
			if fixture.mode != "bridge" && len(environment.calls) != 1 {
				t.Fatal("non-bridge detection mutated networking")
			}
		})
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
			if state == "owned" {
				if len(environment.calls) != 2 || !reflect.DeepEqual(environment.calls[1],
					[]string{"podman", "network", "rm", "exasol-db-test-services"}) {
					t.Fatalf("removal commands = %q", environment.calls)
				}
			} else if len(environment.calls) != 1 {
				t.Fatal("removed absent or foreign network")
			}
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
