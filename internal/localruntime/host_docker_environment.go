// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/winget"
)

const (
	dockerCommand = "docker"
	// dockerProbeTimeout bounds the daemon probe, so a hung daemon cannot
	// hold up `exasol status` past its own timeout.
	dockerProbeTimeout = 10 * time.Second
	dockerLinuxOSType  = "linux"
	// podmanEngineComponent is how Podman names itself among the server
	// components of its Docker-compatible API.
	podmanEngineComponent = "Podman Engine"
	// dockerDesktopDefaultPipe is the Docker API pipe Docker Desktop opens.
	dockerDesktopDefaultPipe = "npipe:////./pipe/docker_engine"
	dockerHostVariable       = "DOCKER_HOST"
	// minimumDockerMajorVersion is the first Docker Engine release that mounts
	// one image at several targets of a container, as Java SLCs need.
	minimumDockerMajorVersion = 29
)

var errDockerUnavailable = errors.New("docker is not available")

// The launcher waits this long for a Docker Desktop it installed to run
// Linux containers. Its first start can include accepting Docker's agreement.
var (
	dockerDesktopReadyTimeout = 10 * time.Minute
	dockerDesktopPollInterval = 2 * time.Second
	startDockerDesktop        = launchDockerDesktop
)

// dockerHostEnvironmentPreparer readies a direct host whose containers run on
// Docker. An installed Docker is used as found, so checking it needs no
// approval. On Windows a missing Docker is installed after approval, and the
// launcher starts that installation once.
type dockerHostEnvironmentPreparer struct {
	platform HostPlatform
}

func (preparer dockerHostEnvironmentPreparer) Platform() HostPlatform {
	return preparer.platform
}

func (preparer dockerHostEnvironmentPreparer) EnsureReady(
	ctx context.Context,
	options PrepareOptions,
) error {
	if preparer.platform == HostPlatformWindows && resolveDocker(ctx, true) != nil {
		if err := installWithWinget(ctx, options, localinstall.EngineDocker); err != nil {
			return err
		}

		return startInstalledDockerDesktop(ctx, options)
	}

	return requireDockerLinuxEngine(
		ctx, preparer.platform == HostPlatformWindows, !options.engineRecorded,
	)
}

// EnsureStartable repeats the readiness check, because the daemon can stop or
// lose its socket between preparation and start.
func (preparer dockerHostEnvironmentPreparer) EnsureStartable(
	ctx context.Context, _, _ io.Writer,
) error {
	return requireDockerLinuxEngine(ctx, preparer.platform == HostPlatformWindows, false)
}

// ContainerHostRunning reports whether the Docker daemon answers. A daemon
// that does not answer is unreachable rather than stopped, because Docker
// restarts the deployment's containers once the daemon is back.
func (preparer dockerHostEnvironmentPreparer) ContainerHostRunning(
	ctx context.Context,
) (bool, error) {
	if err := resolveDocker(ctx, preparer.platform == HostPlatformWindows); err != nil {
		return false, fmt.Errorf("%w: %w", ErrContainerHostUnreachable, dockerUnavailable(err, ""))
	}
	if probe, err := probeDockerDaemon(ctx); err != nil {
		return false, fmt.Errorf(
			"%w: %w", ErrContainerHostUnreachable, dockerUnavailable(err, probe.output),
		)
	}

	return true, nil
}

func (preparer dockerHostEnvironmentPreparer) NewExecutionEnvironment(
	runtimeExec []string,
) localinstall.ExecutionEnvironment {
	direct := localinstall.NewDirectExecutionEnvironment(runtimeExec)
	if preparer.platform == HostPlatformWindows {
		return windowsDockerExecutionEnvironment{DirectExecutionEnvironment: direct}
	}

	return direct
}

// resolveDocker makes the docker CLI resolvable from this process. With
// refresh, it also reads the PATH Windows lists now, because Windows fixes a
// process's PATH at start-up and misses a Docker installed since.
func resolveDocker(ctx context.Context, refresh bool) error {
	_, err := exec.LookPath(dockerCommand)
	if err == nil || !refresh {
		return err
	}
	if refreshErr := winget.RefreshPath(ctx); refreshErr != nil {
		return fmt.Errorf("failed to refresh PATH while looking for docker: %w", refreshErr)
	}
	_, err = exec.LookPath(dockerCommand)

	return err
}

// requireDockerLinuxEngine checks that the docker CLI resolves, its daemon
// answers, and the daemon runs Linux containers on an engine recent enough
// for Nano. A switchable deployment may still select Podman instead, which
// the failure then suggests.
func requireDockerLinuxEngine(ctx context.Context, refreshPath, switchable bool) error {
	if err := resolveDocker(ctx, refreshPath); err != nil {
		return dockerUnavailable(err, "")
	}
	probe, err := probeDockerDaemon(ctx)
	if err != nil {
		return dockerUnavailable(err, probe.output)
	}

	return requireDockerProbeSupported(probe, switchable)
}

func requireDockerProbeSupported(probe dockerProbe, switchable bool) error {
	selectPodman := ""
	if switchable {
		selectPodman = ", or select Podman with `exasol config set --container-engine podman`"
	}
	if probe.podman {
		return fmt.Errorf(
			"%w: the docker command reaches Podman %s, not a Docker daemon; point the "+
				"docker command at a Docker daemon (`docker context ls`)%s",
			errDockerUnavailable, probe.serverVersion, selectPodman,
		)
	}
	if probe.osType != dockerLinuxOSType {
		return fmt.Errorf(
			"%w: the Docker daemon runs %q containers, but local deployments need "+
				"Linux containers; switch Docker to Linux containers%s",
			errDockerUnavailable, probe.osType, selectPodman,
		)
	}
	if !dockerVersionSupported(probe.serverVersion) {
		return fmt.Errorf(
			"%w: the Docker daemon runs Docker Engine %q, but local deployments need "+
				"Docker Engine %d or later; upgrade Docker%s",
			errDockerUnavailable, probe.serverVersion, minimumDockerMajorVersion, selectPodman,
		)
	}

	return nil
}

// startInstalledDockerDesktop starts the Docker Desktop the launcher just
// installed and waits for its daemon to run Linux containers.
func startInstalledDockerDesktop(ctx context.Context, options PrepareOptions) error {
	progress := options.Progress
	minutes := int(dockerDesktopReadyTimeout.Minutes())
	writePreparationProgress(progress, fmt.Sprintf(
		"Starting Docker Desktop. If it asks you to accept Docker's agreement, accept it "+
			"in the Docker Desktop window. Waiting up to %d minutes for Docker to run "+
			"Linux containers; press Ctrl+C to stop waiting...", minutes,
	))
	if err := startDockerDesktop(); err != nil {
		return fmt.Errorf(
			"%w: Docker Desktop was installed but could not be started: %w; "+
				"start Docker Desktop, then re-run this command",
			errDockerUnavailable, err,
		)
	}
	deadline := time.Now().Add(dockerDesktopReadyTimeout)
	for {
		if probe, err := probeDockerDaemon(ctx); err == nil {
			return requireInstalledDockerDesktopProbe(ctx, probe, !options.engineRecorded)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf(
				"%w: Docker Desktop did not run Linux containers within %d minutes; "+
					"finish starting Docker Desktop, accepting its agreement and signing "+
					"out and in again if it asks, then re-run this command",
				errDockerUnavailable, minutes,
			)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(dockerDesktopPollInterval):
		}
	}
}

// requireInstalledDockerDesktopProbe explains a Podman that answers in Docker
// Desktop's place. On Docker's default pipe, Podman stops Docker Desktop from
// starting until it releases the pipe; elsewhere, DOCKER_HOST or the Docker
// context points the docker command away from Docker Desktop.
func requireInstalledDockerDesktopProbe(
	ctx context.Context,
	probe dockerProbe,
	switchable bool,
) error {
	if !probe.podman {
		return requireDockerProbeSupported(probe, switchable)
	}
	endpoint := currentDockerEndpoint(ctx)
	if endpoint.host == "" || strings.EqualFold(endpoint.host, dockerDesktopDefaultPipe) {
		return fmt.Errorf(
			"%w: Podman %s answers on Docker's API pipe, which stops Docker Desktop "+
				"from starting; stop the Podman machine with `podman machine stop`, or "+
				"end its win-sshproxy process if Podman is uninstalled, then start Docker "+
				"Desktop and re-run this command",
			errDockerUnavailable, probe.serverVersion,
		)
	}
	redirect := "switch to Docker Desktop with `docker context use desktop-linux`"
	if endpoint.source == dockerHostVariable {
		redirect = "unset " + dockerHostVariable + " or point it at Docker Desktop"
	}

	return fmt.Errorf(
		"%w: the docker command connects to Podman %s at %s, which %s selects, "+
			"instead of Docker Desktop; %s, then re-run this command",
		errDockerUnavailable, probe.serverVersion, endpoint.host, endpoint.source, redirect,
	)
}

// dockerEndpoint is where the docker command connects and what selected it.
type dockerEndpoint struct {
	host   string
	source string
}

// currentDockerEndpoint reports the docker command's endpoint, or nothing when
// the Docker context cannot be read.
func currentDockerEndpoint(ctx context.Context) dockerEndpoint {
	if host := strings.TrimSpace(os.Getenv(dockerHostVariable)); host != "" {
		return dockerEndpoint{host: host, source: dockerHostVariable}
	}
	probeCtx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	//nolint:gosec // fixed command and arguments
	output, err := exec.CommandContext(
		probeCtx, dockerCommand, "context", "inspect",
		"--format", "{{.Name}} {{.Endpoints.docker.Host}}",
	).Output()
	if err != nil {
		return dockerEndpoint{}
	}
	name, host, found := strings.Cut(strings.TrimSpace(string(output)), " ")
	if !found {
		return dockerEndpoint{}
	}

	return dockerEndpoint{host: host, source: "the Docker context " + name}
}

// launchDockerDesktop starts Docker Desktop without waiting for it, so it
// keeps running after the launcher exits.
func launchDockerDesktop() error {
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	//nolint:gosec,noctx // fixed executable; it must outlive this command
	command := exec.Command(filepath.Join(programFiles, "Docker", "Docker", "Docker Desktop.exe"))
	if err := command.Start(); err != nil {
		return err
	}

	return command.Process.Release()
}

func dockerVersionSupported(serverVersion string) bool {
	major, _, _ := strings.Cut(serverVersion, ".")
	number, err := strconv.Atoi(major)

	return err == nil && number >= minimumDockerMajorVersion
}

// dockerProbe is the daemon's OS type and engine version, and whether Podman
// answers in its place, or the CLI's error output when nothing answers.
type dockerProbe struct {
	osType        string
	serverVersion string
	podman        bool
	output        string
}

func probeDockerDaemon(ctx context.Context) (dockerProbe, error) {
	probeCtx, cancel := context.WithTimeout(ctx, dockerProbeTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	//nolint:gosec // fixed command and arguments
	command := exec.CommandContext(
		probeCtx, dockerCommand, "version", "--format",
		"{{.Server.Os}} {{.Server.Version}} {{range .Server.Components}}{{.Name}};{{end}}",
	)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return dockerProbe{output: strings.TrimSpace(stderr.String())}, err
	}
	osType, rest, _ := strings.Cut(strings.TrimSpace(stdout.String()), " ")
	serverVersion, components, _ := strings.Cut(rest, " ")

	return dockerProbe{
		osType:        osType,
		serverVersion: serverVersion,
		podman:        slices.Contains(strings.Split(components, ";"), podmanEngineComponent),
	}, nil
}

func dockerUnavailable(err error, output string) error {
	detail := err.Error()
	if output != "" {
		detail = output
	}

	return fmt.Errorf(
		"%w: %s\nCheck that the Docker daemon is running (`docker info`), that your "+
			"user may use it (for example membership of the `docker` group, which "+
			"applies to new login sessions), and that the active Docker context "+
			"(`docker context ls`) is the one you expect",
		errDockerUnavailable, detail,
	)
}

// windowsDockerExecutionEnvironment skips Sync: Windows has no host-side
// `sync`, and Docker Desktop's VM offers no supported way to flush it.
type windowsDockerExecutionEnvironment struct {
	*localinstall.DirectExecutionEnvironment
}

// Run resolves docker before delegating, so commands that never call Prepare
// find a Docker installed after this terminal was opened.
func (environment windowsDockerExecutionEnvironment) Run(
	ctx context.Context,
	stdin io.Reader,
	stdout, stderr io.Writer,
	command ...string,
) error {
	if len(command) > 0 && command[0] == dockerCommand {
		if err := resolveDocker(ctx, true); err != nil {
			return dockerUnavailable(err, "")
		}
	}

	return environment.DirectExecutionEnvironment.Run(ctx, stdin, stdout, stderr, command...)
}

func (windowsDockerExecutionEnvironment) Sync(context.Context, io.Writer, io.Writer) error {
	return nil
}
