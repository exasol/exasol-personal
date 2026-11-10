// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/winget"
)

const linuxDockerShim = `case "$1" in
  version) printf 'linux 29.0.1 Engine;containerd;runc;\n' ;;
  --version) printf 'Docker version 29.0.1\n' ;;
esac`

// fakeDockerDesktop stands in for the winget installation and the start of
// Docker Desktop, which tests cannot run.
type fakeDockerDesktop struct {
	installed []winget.Package
	started   int
}

func useFakeDockerDesktop(
	t *testing.T,
	shimDir, dockerBody string,
	readyTimeout time.Duration,
) *fakeDockerDesktop {
	t.Helper()
	fake := &fakeDockerDesktop{}
	originalInstall := installWingetPackage
	originalStart := startDockerDesktop
	originalTimeout := dockerDesktopReadyTimeout
	originalInterval := dockerDesktopPollInterval
	t.Cleanup(func() {
		installWingetPackage = originalInstall
		startDockerDesktop = originalStart
		dockerDesktopReadyTimeout = originalTimeout
		dockerDesktopPollInterval = originalInterval
	})
	installWingetPackage = func(_ context.Context, pkg winget.Package, _, _ io.Writer) error {
		fake.installed = append(fake.installed, pkg)
		dropShim(t, shimDir, "docker", dockerBody)

		return nil
	}
	startDockerDesktop = func() error {
		fake.started++

		return nil
	}
	dockerDesktopReadyTimeout = readyTimeout
	dockerDesktopPollInterval = time.Millisecond

	return fake
}

//nolint:paralleltest // The test replaces process-wide PATH and package hooks.
func TestWindowsDockerPrepare_InstallsAndStartsDockerDesktop(t *testing.T) {
	clearPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "winget", `exit 0`)
	fake := useFakeDockerDesktop(t, dir, linuxDockerShim, time.Minute)
	deployment := config.NewDeploymentDir(t.TempDir())
	approver := &recordingApprover{approve: true}

	err := NewHostWindowsRuntime(deployment, nil, localinstall.EngineDocker).
		Prepare(context.Background(), nil, nil, approver.options())
	if err != nil {
		t.Fatalf("Prepare() unexpected error: %v", err)
	}
	if len(approver.requests) != 1 || approver.requests[0].Engine != localinstall.EngineDocker ||
		!strings.Contains(approver.requests[0].Commands[0].String(), "--id Docker.DockerDesktop") {
		t.Fatalf("expected approval of the Docker Desktop installation, got %+v", approver.requests)
	}
	if len(fake.installed) != 1 || fake.installed[0] != winget.DockerDesktopPackage {
		t.Errorf("expected Docker Desktop to be installed, got %v", fake.installed)
	}
	if fake.started != 1 {
		t.Errorf("expected Docker Desktop to be started once, got %d", fake.started)
	}
	if !strings.Contains(approver.progress.String(), "accept it in the Docker Desktop window") {
		t.Errorf("expected directions for Docker's agreement, got %q", approver.progress.String())
	}
	engine, recorded, err := RecordedContainerEngine(deployment)
	if err != nil || !recorded || engine != localinstall.EngineDocker {
		t.Errorf("expected Docker to be recorded, got %q %v %v", engine, recorded, err)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH and package hooks.
func TestWindowsDockerPrepare_DeclinedInstallationInstallsNothing(t *testing.T) {
	clearPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "winget", `exit 0`)
	fake := useFakeDockerDesktop(t, dir, linuxDockerShim, time.Minute)
	deployment := config.NewDeploymentDir(t.TempDir())

	err := NewHostWindowsRuntime(deployment, nil, localinstall.EngineDocker).
		Prepare(context.Background(), nil, nil, (&recordingApprover{}).options())

	if err == nil || !strings.Contains(err.Error(), "was not approved") {
		t.Fatalf("expected the declined installation to fail, got %v", err)
	}
	if len(fake.installed) != 0 || fake.started != 0 {
		t.Errorf("expected nothing installed or started, got %v and %d starts",
			fake.installed, fake.started)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH and package hooks.
func TestWindowsDockerPrepare_ExplainsDockerDesktopThatDoesNotBecomeReady(t *testing.T) {
	clearPathOverride(t)
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "winget", `exit 0`)
	useFakeDockerDesktop(t, dir, `printf 'daemon is starting\n' >&2; exit 1`, 0)
	deployment := config.NewDeploymentDir(t.TempDir())
	approver := &recordingApprover{approve: true}

	err := NewHostWindowsRuntime(deployment, nil, localinstall.EngineDocker).
		Prepare(context.Background(), nil, nil, approver.options())

	if !errors.Is(err, errDockerUnavailable) ||
		!strings.Contains(err.Error(), "finish starting Docker Desktop") ||
		!strings.Contains(err.Error(), "re-run this command") {
		t.Fatalf("expected directions to finish starting Docker Desktop, got %v", err)
	}
	if _, recorded, _ := RecordedContainerEngine(deployment); recorded {
		t.Error("expected no engine to be recorded before Docker is ready")
	}
}

// podmanDockerShim answers the docker command as Podman would, through the
// Docker context named in context.
func podmanDockerShim(contextLine string) string {
	return `case "$1" in
  context) printf '` + contextLine + `\n' ;;
  *) printf 'linux 5.8.5 Podman Engine;Conmon;Engine;\n' ;;
esac`
}

// Podman answering in Docker Desktop's place is explained by where the docker
// command connects: only Podman on Docker's default pipe stops Docker Desktop
// from starting, and waiting cannot help either way.
//
//nolint:paralleltest // The tests replace process-wide PATH and package hooks.
func TestWindowsDockerPrepare_ExplainsPodmanAnsweringForDocker(t *testing.T) {
	tests := []struct {
		name       string
		context    string
		dockerHost string
		want       []string
	}{
		{
			name:    "podman holds the default pipe",
			context: "default npipe:////./pipe/docker_engine",
			want:    []string{"Podman 5.8.5 answers on Docker's API pipe", "podman machine stop"},
		},
		{
			name:    "a docker context selects podman",
			context: "podman npipe:////./pipe/podman-machine-default",
			want: []string{
				"npipe:////./pipe/podman-machine-default, which the Docker context podman selects",
				"docker context use desktop-linux",
			},
		},
		{
			name:       "DOCKER_HOST selects podman",
			context:    "default npipe:////./pipe/docker_engine",
			dockerHost: "npipe:////./pipe/podman-machine-default",
			want:       []string{"which DOCKER_HOST selects", "unset DOCKER_HOST"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearPathOverride(t)
			t.Setenv("DOCKER_HOST", test.dockerHost)
			dir := newIsolatedShimDir(t)
			dropShim(t, dir, "winget", `exit 0`)
			useFakeDockerDesktop(t, dir, podmanDockerShim(test.context), time.Hour)
			approver := &recordingApprover{approve: true}

			err := NewHostWindowsRuntime(
				config.NewDeploymentDir(t.TempDir()),
				nil,
				localinstall.EngineDocker,
			).
				Prepare(context.Background(), nil, nil, approver.options())

			if !errors.Is(err, errDockerUnavailable) {
				t.Fatalf("expected Docker to be unavailable, got %v", err)
			}
			for _, part := range test.want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("expected %q in %v", part, err)
				}
			}
		})
	}
}

// A deployment that recorded its engine cannot select another, so neither
// the installation request nor Docker's readiness failure suggests one.
//
//nolint:paralleltest // The tests replace process-wide PATH with fake binary shims.
func TestPrepare_RecordedDeploymentIsNotOfferedTheOtherEngine(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		t.Run(fmt.Sprintf("recorded=%t", recorded), func(t *testing.T) {
			t.Setenv("EXASOL_WINGET_TEST_PATH_ADDITIONS", "")
			dir := newIsolatedShimDir(t)
			dropShim(t, dir, "winget", `exit 0`)
			podmanDeployment := config.NewDeploymentDir(t.TempDir())
			dockerDeployment := config.NewDeploymentDir(t.TempDir())
			if recorded {
				writeLinuxRuntimeTestFile(t,
					containerEngineMarkerPath(newRuntimePaths(podmanDeployment)),
					`{"engine":"podman"}`)
				writeLinuxRuntimeTestFile(t,
					containerEngineMarkerPath(newRuntimePaths(dockerDeployment)),
					`{"engine":"docker"}`)
			}
			approver := &recordingApprover{}

			_ = NewHostWindowsRuntime(podmanDeployment, nil, localinstall.EnginePodman).
				Prepare(context.Background(), nil, nil, approver.options())
			dropShim(t, dir, "docker", `printf 'linux 28.0.4\n'`)
			dockerErr := NewHostLinuxRuntime(dockerDeployment, nil, localinstall.EngineDocker).
				Prepare(context.Background(), nil, nil, PrepareOptions{})

			if len(approver.requests) != 1 {
				t.Fatalf("expected one installation request, got %+v", approver.requests)
			}
			suggestsDocker := approver.requests[0].Alternative == localinstall.EngineDocker
			suggestsPodman := strings.Contains(dockerErr.Error(), "--container-engine podman")
			if suggestsDocker == recorded || suggestsPodman == recorded {
				t.Errorf("expected the other engine to be suggested only when unrecorded, "+
					"got alternative %q and %v", approver.requests[0].Alternative, dockerErr)
			}
		})
	}
}

// Windows fixes a process's PATH at start-up, so a terminal opened before
// Docker Desktop was installed must still find it.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestWindowsDockerPrepare_FindsDockerInstalledAfterTheTerminalOpened(t *testing.T) {
	dir := newIsolatedShimDir(t)
	wingetLog := dropShim(t, dir, "winget", `exit 1`)
	dockerDir := t.TempDir()
	dropShim(t, dockerDir, "docker", linuxDockerShim)
	t.Setenv("EXASOL_WINGET_TEST_PATH_ADDITIONS", dockerDir)
	approver := &recordingApprover{approve: true}

	err := NewHostWindowsRuntime(
		config.NewDeploymentDir(t.TempDir()),
		nil,
		localinstall.EngineDocker,
	).
		Prepare(context.Background(), nil, nil, approver.options())
	if err != nil {
		t.Fatalf("Prepare() unexpected error: %v", err)
	}
	if len(approver.requests) != 0 {
		t.Errorf("expected no installation request, got %+v", approver.requests)
	}
	if logged := readFileIfExists(t, wingetLog); logged != "" {
		t.Errorf("expected winget not to run, got %q", logged)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestContainerEngineInstallOptions_ListEnginesInOrderWithTheirCommands(t *testing.T) {
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "winget", `exit 0`)

	options, err := ContainerEngineInstallOptions(
		[]localinstall.ContainerEngine{localinstall.EnginePodman, localinstall.EngineDocker},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(options) != 2 ||
		options[0].Engine != localinstall.EnginePodman ||
		!strings.Contains(options[0].Commands[0].String(), "--id RedHat.Podman") ||
		options[1].Engine != localinstall.EngineDocker ||
		!strings.Contains(options[1].Commands[0].String(), "--id Docker.DockerDesktop") {
		t.Fatalf("unexpected options %+v", options)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestContainerEngineInstallOptions_ExplainMissingWinget(t *testing.T) {
	newIsolatedShimDir(t)

	_, err := ContainerEngineInstallOptions(
		[]localinstall.ContainerEngine{localinstall.EnginePodman},
	)

	if err == nil || !strings.Contains(err.Error(), "App Installer") ||
		!strings.Contains(err.Error(), "Docker Desktop") {
		t.Fatalf("expected guidance to install winget or an engine manually, got %v", err)
	}
}

func TestWithInstallationApproved_ApprovesOnlyTheChosenEngine(t *testing.T) {
	t.Parallel()

	approver := &recordingApprover{}
	options := approver.options().WithInstallationApproved(localinstall.EngineDocker)

	chosen, err := options.ApproveHostChange(context.Background(), HostChangeRequest{
		Kind: HostChangeInstallContainerRuntime, Engine: localinstall.EngineDocker,
	})
	if err != nil || !chosen {
		t.Errorf("expected the chosen engine's installation to be approved, got %v %v", chosen, err)
	}
	other, err := options.ApproveHostChange(context.Background(), HostChangeRequest{
		Kind: HostChangeInstallContainerRuntime, Engine: localinstall.EnginePodman,
	})
	if err != nil || other {
		t.Errorf("expected another installation to be asked about, got %v %v", other, err)
	}
	if len(approver.requests) != 1 || approver.requests[0].Engine != localinstall.EnginePodman {
		t.Errorf("expected only the other installation to reach the approver, got %+v",
			approver.requests)
	}
}

func readFileIfExists(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestInstallWithWinget_SuggestsOnlyAnAlternativeThatCanRun(t *testing.T) {
	tests := []struct {
		name     string
		docker   string
		expected localinstall.ContainerEngine
	}{
		{name: "docker missing", expected: localinstall.EngineDocker},
		{
			name:     "docker runs linux containers",
			docker:   linuxDockerShim,
			expected: localinstall.EngineDocker,
		},
		{name: "docker runs windows containers", docker: `printf 'windows 29.0.1 Engine;\n'`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := newIsolatedShimDir(t)
			dropShim(t, dir, "winget", `exit 0`)
			t.Setenv("EXASOL_WINGET_TEST_PATH_ADDITIONS", "")
			if test.docker != "" {
				dropShim(t, dir, "docker", test.docker)
			}
			approver := &recordingApprover{}

			_ = installWithWinget(context.Background(), approver.options(),
				localinstall.EnginePodman)

			if len(approver.requests) != 1 ||
				approver.requests[0].Alternative != test.expected {
				t.Fatalf("expected alternative %q, got %+v", test.expected, approver.requests)
			}
		})
	}
}
