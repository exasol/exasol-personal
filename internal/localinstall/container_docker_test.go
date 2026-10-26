// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// scriptedRunner answers each command from the first reply whose prefix it
// starts with, and records every command it receives.
type scriptedRunner struct {
	replies []scriptedReply
	calls   []string
}

type scriptedReply struct {
	prefix string
	output string
	stderr string
	err    error
}

func (runner *scriptedRunner) Run(
	_ context.Context, _ io.Reader, stdout, stderr io.Writer, command ...string,
) error {
	line := strings.Join(command, " ")
	runner.calls = append(runner.calls, line)
	for _, reply := range runner.replies {
		if strings.HasPrefix(line, reply.prefix) {
			if stdout != nil {
				_, _ = io.WriteString(stdout, reply.output)
			}
			if stderr != nil {
				_, _ = io.WriteString(stderr, reply.stderr)
			}

			return reply.err
		}
	}

	return nil
}

func (runner *scriptedRunner) called(prefix string) bool {
	return slices.ContainsFunc(runner.calls, func(call string) bool {
		return strings.HasPrefix(call, prefix)
	})
}

func nanoLikeSpec() ContainerSpec {
	return ContainerSpec{
		Name:            "exasol-db-test",
		Image:           "exasol/nano:test",
		Replace:         true,
		NestedSandbox:   true,
		SecurityOptions: []string{"label=disable"},
		Entrypoint:      []string{"/bin/server", "--verbose"},
		Args:            []string{"init"},
		Mounts: []Mount{
			{Kind: MountImage, Source: "slc:1", Target: "/exa/slc/python", Writable: true},
		},
	}
}

// dockerRuntimeOnHost is the Docker runtime on a host that restricts user
// namespaces as given.
func dockerRuntimeOnHost(runner CommandRunner, restricted bool) dockerRuntime {
	return dockerRuntime{podmanRuntime{
		environment: runner, binary: dockerBinary, dialect: dockerDialect,
	}, func() bool { return restricted }}
}

func TestDockerCreate_SpellsSpecForDocker(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker ps", output: "abc\n"},
		{prefix: "docker container inspect", output: `[{"Id":"abc","Name":"/exasol-db-test"}]`},
	}}

	err := dockerRuntimeOnHost(runner, false).Create(context.Background(), nanoLikeSpec(), nil, nil)

	require.NoError(t, err)
	require.True(t, runner.called("docker rm --force -- exasol-db-test"))
	require.False(t, runner.called("docker info"))
	run := runner.calls[len(runner.calls)-1]
	require.NotContains(t, run, "--replace")
	require.NotContains(t, run, "--cap-add")
	require.Contains(
		t,
		run,
		"--security-opt seccomp=unconfined --security-opt apparmor=unconfined "+
			"--security-opt systempaths=unconfined --security-opt label=disable",
	)
	require.Contains(t, run, "--mount type=image,source=slc:1,destination=/exa/slc/python ")
	require.True(t, strings.HasSuffix(run,
		"--entrypoint /bin/server exasol/nano:test --verbose init"), run)
}

func TestDockerCreate_GrantsSysAdminToTheSandboxUnderUserNamespaceRestriction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		restricted      bool
		securityOptions string
		wantSysAdmin    bool
	}{
		{
			name: "rootful daemon with AppArmor", restricted: true,
			securityOptions: `["name=apparmor","name=seccomp,profile=builtin","name=cgroupns"]`,
			wantSysAdmin:    true,
		},
		{
			name: "host without the restriction", restricted: false,
			securityOptions: `["name=apparmor","name=seccomp,profile=builtin"]`,
		},
		{
			name: "rootless daemon", restricted: true,
			securityOptions: `["name=seccomp,profile=builtin","name=rootless","name=cgroupns"]`,
		},
		{
			name: "daemon without AppArmor", restricted: true,
			securityOptions: `["name=seccomp,profile=builtin","name=cgroupns"]`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := &scriptedRunner{replies: []scriptedReply{
				{prefix: "docker info", output: test.securityOptions + "\n"},
			}}

			err := dockerRuntimeOnHost(runner, test.restricted).
				Create(context.Background(), nanoLikeSpec(), nil, nil)

			require.NoError(t, err)
			run := runner.calls[len(runner.calls)-1]
			if test.wantSysAdmin {
				require.Contains(t, run, "--cap-add SYS_ADMIN")
			} else {
				require.NotContains(t, run, "--cap-add")
			}
		})
	}
}

func TestDockerCreate_ReportsUnreadableSecurityOptions(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker info", stderr: "daemon gone", err: errors.New("exit status 1")},
	}}
	var outErr bytes.Buffer

	err := dockerRuntimeOnHost(runner, true).
		Create(context.Background(), nanoLikeSpec(), nil, &outErr)

	require.Error(t, err)
	require.Contains(t, outErr.String(), "daemon gone")
	require.False(t, runner.called("docker run"))
}

func TestDockerCreate_SandboxlessContainerSkipsTheDaemonProbe(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{}
	spec := ContainerSpec{Name: "sidecar", Image: "busybox"}

	err := dockerRuntimeOnHost(runner, true).Create(context.Background(), spec, nil, nil)

	require.NoError(t, err)
	require.False(t, runner.called("docker info"))
}

func TestPodmanCreate_KeepsPodmanSpelling(t *testing.T) {
	t.Parallel()

	args, err := createArgs(nanoLikeSpec(), podmanDialect, true)

	require.NoError(t, err)
	run := strings.Join(args, " ")
	require.Contains(t, run, "run --detach --replace --name exasol-db-test")
	require.Contains(t, run, "--security-opt unmask=ALL --security-opt label=disable")
	require.Contains(t, run, "type=image,source=slc:1,destination=/exa/slc/python,rw=true")
	require.True(t, strings.HasSuffix(run,
		`--entrypoint ["/bin/server","--verbose"] exasol/nano:test init`), run)
}

func TestDockerRunToCompletion_RemovesContainerAfterExit(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{}

	err := newDockerRuntime(runner).RunToCompletion(
		context.Background(), ContainerSpec{Image: "busybox", Args: []string{"true"}}, nil, nil,
	)

	require.NoError(t, err)
	require.Equal(t, []string{"docker run --rm busybox true"}, runner.calls)
}

func TestDockerRemove_RemovesContainerByID(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{}

	err := newDockerRuntime(runner).Remove(context.Background(), "565b4d86", true, nil, nil)

	require.NoError(t, err)
	require.Equal(t, []string{"docker rm --force -- 565b4d86"}, runner.calls)
}

func TestDockerRemove_AcceptsAbsentContainer(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{{
		prefix: "docker rm",
		stderr: "Error response from daemon: No such container: gone\n",
		err:    errors.New("exit status 1"),
	}}}

	err := newDockerRuntime(runner).Remove(context.Background(), "gone", true, nil, nil)

	require.NoError(t, err)
}

func TestDockerRemove_ReportsOtherFailures(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{{
		prefix: "docker rm",
		stderr: "permission denied while trying to connect to the docker API\n",
		err:    errors.New("exit status 1"),
	}}}

	err := newDockerRuntime(runner).Remove(context.Background(), "db", true, nil, nil)

	require.Error(t, err)
}

func TestDockerList_ReadsInspectedContainers(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker ps", output: "abc\n"},
		{prefix: "docker container inspect abc", output: `[{
			"Id": "abc", "Name": "/svc",
			"State": {"Status": "exited", "ExitCode": 3, "StartedAt": "2026-10-28T09:15:00.5Z"},
			"Config": {"Labels": {"owner": "me"}},
			"NetworkSettings": {"Ports": {
				"4896/tcp": [{"HostIp": "127.0.0.1", "HostPort": "4896"}]
			}}
		}]`},
	}}

	containers, err := newDockerRuntime(runner).List(
		context.Background(), nil, ContainerFilter{Label: "owner=me"},
	)

	require.NoError(t, err)
	require.Equal(t, []ContainerListing{{
		ID: "abc", Names: []string{"svc"}, Labels: map[string]string{"owner": "me"},
		State: "exited", Lifecycle: ContainerLifecycleStopped, ExitCode: 3,
		Ports: []ContainerPort{{
			HostIP: "127.0.0.1", HostPort: 4896, ContainerPort: 4896, Protocol: "tcp",
		}},
		StartedAt: time.Date(2026, 10, 28, 9, 15, 0, 500000000, time.UTC),
	}}, containers)
	require.True(t, runner.called("docker ps --all --quiet --no-trunc --filter label=owner=me"))
}

// Regression: Docker's `restarting` read as stopped, so `exasol stop` recorded a
// deployment as stopped and left Nano for Docker to restart.
func TestContainerLifecycle_KeepsEngineManagedContainersActive(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		engine  string
		state   string
		running bool
		active  bool
	}{
		{engine: "docker", state: "running", running: true, active: true},
		{engine: "docker", state: "restarting", active: true},
		{engine: "docker", state: "paused", active: true},
		{engine: "docker", state: "exited"},
		{engine: "docker", state: "created"},
		{engine: "docker", state: "dead"},
		{engine: "docker", state: "something-new", active: true},
		{engine: "podman", state: "running", running: true, active: true},
		{engine: "podman", state: "paused", active: true},
		{engine: "podman", state: "exited"},
		{engine: "podman", state: "stopping"},
		{engine: "podman", state: "configured"},
		{engine: "podman", state: "something-new", active: true},
	} {
		t.Run(test.engine+"/"+test.state, func(t *testing.T) {
			t.Parallel()
			listing := podmanListing{State: test.state}.container()
			if test.engine == "docker" {
				listing = dockerInspection{State: dockerState{Status: test.state}}.listing()
			}

			require.Equal(t, test.running, listing.Running(), "running")
			require.Equal(t, test.active, listing.Active(), "active")
			require.Equal(t, test.state, listing.State, "reported state")
		})
	}
}

func TestDockerNetworks_ReportUserBridgesWithDNS(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker network ls", output: "n1\nn2\n"},
		{prefix: "docker network inspect", output: `[
			{"Name": "bridge", "Driver": "bridge"},
			{"Name": "owned-services", "Driver": "bridge", "Labels": {"owner": "me"}}
		]`},
	}}

	networks, err := newDockerRuntime(runner).Networks(context.Background(), nil)

	require.NoError(t, err)
	require.Equal(t, []ContainerNetwork{
		{Name: "bridge", Bridge: true, DNSEnabled: false},
		{
			Name:       "owned-services",
			Bridge:     true,
			DNSEnabled: true,
			Labels:     map[string]string{"owner": "me"},
		},
	}, networks)
}

// Regression: another deployment removing its network between `docker network
// ls` and the inspection failed this deployment's start and destroy.
func TestDockerNetworks_SkipNetworksRemovedSinceListing(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker network ls", output: "n1\ngone\n"},
		{
			prefix: "docker network inspect n1 gone",
			stderr: "Error response from daemon: network gone not found\n",
			err:    errors.New("exit status 1"),
		},
		{prefix: "docker network inspect n1", output: `[{"Name": "bridge", "Driver": "bridge"}]`},
		{
			prefix: "docker network inspect gone",
			stderr: "Error response from daemon: network gone not found\n",
			err:    errors.New("exit status 1"),
		},
	}}

	networks, err := newDockerRuntime(runner).Networks(context.Background(), nil)

	require.NoError(t, err)
	require.Equal(t, []ContainerNetwork{{Name: "bridge", Bridge: true}}, networks)
}

func TestDockerNetworks_ReportOtherInspectionFailures(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker network ls", output: "n1\n"},
		{
			prefix: "docker network inspect n1",
			stderr: "permission denied while trying to connect to the docker API\n",
			err:    errors.New("exit status 1"),
		},
	}}
	var outErr bytes.Buffer

	_, err := newDockerRuntime(runner).Networks(context.Background(), &outErr)

	require.Error(t, err)
	require.Contains(t, outErr.String(), "permission denied")
}

// Regression: a container removed between `docker ps` and the inspection, for
// example by a concurrent `exasol stop`, failed the listing.
func TestDockerList_SkipsContainersRemovedSinceListing(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker ps", output: "abc\ngone\n"},
		{
			prefix: "docker container inspect abc gone",
			stderr: "Error response from daemon: No such container: gone\n",
			err:    errors.New("exit status 1"),
		},
		{prefix: "docker container inspect abc", output: `[{"Id": "abc", "Name": "/svc"}]`},
		{
			prefix: "docker container inspect gone",
			stderr: "Error response from daemon: No such container: gone\n",
			err:    errors.New("exit status 1"),
		},
	}}

	containers, err := newDockerRuntime(runner).List(context.Background(), nil)

	require.NoError(t, err)
	require.Len(t, containers, 1)
	require.Equal(t, "abc", containers[0].ID)
}

func TestDockerCreateNetwork_AcceptsExistingNetwork(t *testing.T) {
	t.Parallel()
	existing := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker network inspect svc", output: `[{"Name": "svc", "Driver": "bridge"}]`},
	}}
	missing := &scriptedRunner{replies: []scriptedReply{{
		prefix: "docker network inspect svc",
		stderr: "Error response from daemon: network svc not found\n",
		err:    errors.New("exit status 1"),
	}}}
	spec := NetworkSpec{Name: "svc", DNS: true, Labels: map[string]string{"owner": "me"}}

	require.NoError(
		t,
		newDockerRuntime(existing).CreateNetwork(context.Background(), spec, nil, nil),
	)
	require.NoError(
		t,
		newDockerRuntime(missing).CreateNetwork(context.Background(), spec, nil, nil),
	)

	require.False(t, existing.called("docker network create"))
	require.True(t, missing.called("docker network create --driver bridge --label owner=me svc"))
}

func TestDockerAttachment_TreatsUserNetworkModeAsBridged(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker ps", output: "abc\n"},
		{prefix: "docker container inspect", output: `[{
			"Id": "abc", "Name": "/db",
			"HostConfig": {"NetworkMode": "db-services"},
			"NetworkSettings": {"Networks": {"db-services": {"Aliases": ["exasol-db"]}}}
		}]`},
	}}

	attachment, present, err := newDockerRuntime(runner).Attachment(context.Background(), "db", nil)

	require.NoError(t, err)
	require.True(t, present)
	require.True(t, attachment.Bridged)
	membership, joined := attachment.Member("db-services")
	require.True(t, joined)
	require.Equal(t, []string{"exasol-db"}, membership.Aliases)
}

// Regression: Docker's image listing found nothing for a reference with a
// digest, so a present image read as missing.
func TestDockerImageExists_InspectsTheImage(t *testing.T) {
	t.Parallel()
	const pinned = "docker.io/exasol/mcp-server:2.2.0@sha256:2f27"
	present := &scriptedRunner{replies: []scriptedReply{
		{prefix: "docker image inspect --format {{.Id}} -- " + pinned, output: "sha256:abc\n"},
	}}

	found, err := newDockerRuntime(present).ImageExists(context.Background(), pinned, nil)
	require.NoError(t, err)
	require.True(t, found)

	missing := &scriptedRunner{replies: []scriptedReply{{
		prefix: "docker image inspect", err: errors.New("exit status 1"),
		stderr: "Error response from daemon: No such image: " + pinned + "\n",
	}}}
	found, err = newDockerRuntime(missing).ImageExists(context.Background(), pinned, nil)
	require.NoError(t, err)
	require.False(t, found)

	unreachable := &scriptedRunner{replies: []scriptedReply{{
		prefix: "docker image inspect", err: errors.New("exit status 1"),
		stderr: "Cannot connect to the Docker daemon\n",
	}}}
	_, err = newDockerRuntime(unreachable).ImageExists(context.Background(), pinned, nil)
	require.Error(t, err)
}

func TestParseLoadedImage_AcceptsUntaggedImageID(t *testing.T) {
	t.Parallel()

	image, err := parseLoadedImage("Loaded image ID: sha256:abc\n")

	require.NoError(t, err)
	require.Equal(t, "sha256:abc", image)
}
