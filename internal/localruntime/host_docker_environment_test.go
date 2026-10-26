// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
)

//nolint:paralleltest // The tests replace process-wide PATH with fake binary shims.
func TestDockerPreparer_EnsureReadyChecksTheDaemon(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "linux daemon", body: `printf 'linux 29.9.1\n'`},
		{
			name: "windows containers", body: `printf 'windows 29.9.1\n'`,
			wantErr: "local deployments need Linux containers",
		},
		{
			name: "engine without image mounts at several targets", body: `printf 'linux 28.0.4\n'`,
			wantErr: `Docker Engine "28.0.4", but local deployments need Docker Engine 29 or later`,
		},
		{
			name: "unrecognised engine version", body: `printf 'linux dev\n'`,
			wantErr: "Docker Engine 29 or later",
		},
		{
			name: "daemon unreachable", body: `printf 'permission denied\n' >&2; exit 1`,
			wantErr: "permission denied",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := newIsolatedShimDir(t)
			dropShim(t, dir, "docker", test.body)

			err := dockerHostEnvironmentPreparer{platform: HostPlatformLinux}.
				EnsureReady(context.Background(), PrepareOptions{})

			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("expected Docker to be ready, got %v", err)
				}

				return
			}
			if !errors.Is(err, errDockerUnavailable) ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected an unavailable error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestDockerPreparer_EnsureReadyExplainsMissingDocker(t *testing.T) {
	newIsolatedShimDir(t)

	err := dockerHostEnvironmentPreparer{platform: HostPlatformLinux}.
		EnsureReady(context.Background(), PrepareOptions{})

	if !errors.Is(err, errDockerUnavailable) || !strings.Contains(err.Error(), "docker context") {
		t.Fatalf("expected guidance for an unavailable Docker, got %v", err)
	}
}

//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestDockerPreparer_ContainerHostRunningReflectsTheDaemon(t *testing.T) {
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "docker", `printf 'linux 29.9.1\n'`)

	running, err := dockerHostEnvironmentPreparer{platform: HostPlatformLinux}.
		ContainerHostRunning(context.Background())

	if err != nil || !running {
		t.Fatalf("expected a running container host, got %t (err=%v)", running, err)
	}
}

// Regression: a daemon that did not answer read as stopped, so `exasol stop`
// recorded a deployment as stopped that Docker restarted once it was back.
//
//nolint:paralleltest // The test replaces process-wide PATH with fake binary shims.
func TestDockerPreparer_UnansweringDaemonIsUnreachable(t *testing.T) {
	dir := newIsolatedShimDir(t)
	dropShim(t, dir, "docker", `printf 'Cannot connect to the Docker daemon\n' >&2; exit 1`)

	_, err := dockerHostEnvironmentPreparer{platform: HostPlatformLinux}.
		ContainerHostRunning(context.Background())

	if !errors.Is(err, ErrContainerHostUnreachable) ||
		!strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
		t.Fatalf("expected an unreachable container host naming the cause, got %v", err)
	}
}

func TestDockerPreparer_WindowsSkipsSync(t *testing.T) {
	t.Parallel()
	environment := dockerHostEnvironmentPreparer{platform: HostPlatformWindows}.
		NewExecutionEnvironment([]string{"/nonexistent/engine"})

	if err := environment.Sync(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected Sync to succeed without running a command, got %v", err)
	}
}

func TestHostRuntimes_KeepPodmanPreparationForPodman(t *testing.T) {
	t.Parallel()
	deployment := config.NewDeploymentDir(t.TempDir())

	tests := []struct {
		name     string
		runtime  *HostRuntime
		expected hostRuntimeEnvironmentPreparer
	}{
		{
			name:     "linux podman",
			runtime:  NewHostLinuxRuntime(deployment, nil, localinstall.EnginePodman),
			expected: linuxHostEnvironmentPreparer{},
		},
		{
			name:     "windows podman",
			runtime:  NewHostWindowsRuntime(deployment, nil, localinstall.EnginePodman),
			expected: windowsHostEnvironmentPreparer{},
		},
		{
			name:     "linux docker",
			runtime:  NewHostLinuxRuntime(deployment, nil, localinstall.EngineDocker),
			expected: dockerHostEnvironmentPreparer{platform: HostPlatformLinux},
		},
		{
			name:     "windows docker",
			runtime:  NewHostWindowsRuntime(deployment, nil, localinstall.EngineDocker),
			expected: dockerHostEnvironmentPreparer{platform: HostPlatformWindows},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.runtime.preparer != test.expected {
				t.Fatalf("expected preparer %#v, got %#v", test.expected, test.runtime.preparer)
			}
		})
	}
}

func TestPrepareDockerDataDir_OpensExistingPrivateDirectory(t *testing.T) {
	t.Parallel()
	skipPOSIXPermissionTest(t)
	dataDir := filepath.Join(t.TempDir(), "exa")
	if err := os.Mkdir(dataDir, 0o750); err != nil {
		t.Fatalf("failed to create data directory: %v", err)
	}

	if err := prepareDockerDataDir(dataDir); err != nil {
		t.Fatalf("expected data directory preparation to succeed, got %v", err)
	}
	assertDirMode(t, dataDir, dockerDataDirMode)
}

func TestPrepareDockerDataDir_CreatesTraversableStagingDirectories(t *testing.T) {
	t.Parallel()
	skipPOSIXPermissionTest(t)
	dataDir := filepath.Join(t.TempDir(), "exa")

	if err := prepareDockerDataDir(dataDir); err != nil {
		t.Fatalf("expected data directory preparation to succeed, got %v", err)
	}
	for _, path := range []string{
		filepath.Join(dataDir, "bucketfs"),
		filepath.Join(dataDir, "bucketfs", "bfsdefault"),
		filepath.Join(dataDir, "bucketfs", "bfsdefault", "default"),
		filepath.Join(dataDir, "jdbc"),
	} {
		assertDirMode(t, path, dockerDataDirMode)
	}
}

func TestPrepareDockerDataDir_LeavesExistingStagingDirectoriesUnchanged(t *testing.T) {
	t.Parallel()
	skipPOSIXPermissionTest(t)
	dataDir := filepath.Join(t.TempDir(), "exa")
	jdbcDir := filepath.Join(dataDir, "jdbc")
	if err := os.MkdirAll(jdbcDir, 0o700); err != nil {
		t.Fatalf("failed to create staging directory: %v", err)
	}
	//nolint:gosec // directory mode under test
	if err := os.Chmod(jdbcDir, 0o700); err != nil {
		t.Fatalf("failed to restrict staging directory: %v", err)
	}

	if err := prepareDockerDataDir(dataDir); err != nil {
		t.Fatalf("expected data directory preparation to succeed, got %v", err)
	}
	assertDirMode(t, jdbcDir, 0o700)
}

func TestPrepareDockerDataDir_ToleratesStagingParentOwnedByDatabase(t *testing.T) {
	t.Parallel()
	skipPOSIXPermissionTest(t)
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the directory permission this test relies on")
	}
	dataDir := filepath.Join(t.TempDir(), "exa")
	bucketFSDir := filepath.Join(dataDir, "bucketfs")
	if err := os.MkdirAll(bucketFSDir, 0o750); err != nil {
		t.Fatalf("failed to create BucketFS directory: %v", err)
	}
	//nolint:gosec // directory mode under test
	if err := os.Chmod(bucketFSDir, 0o555); err != nil {
		t.Fatalf("failed to make BucketFS directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bucketFSDir, 0o750) }) //nolint:gosec // restores the fixture

	if err := prepareDockerDataDir(dataDir); err != nil {
		t.Fatalf("expected data directory preparation to succeed, got %v", err)
	}
	assertDirMode(t, filepath.Join(dataDir, "jdbc"), dockerDataDirMode)
}

func skipPOSIXPermissionTest(t *testing.T) {
	t.Helper()
	if os.PathSeparator == '\\' {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
}

func assertDirMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %s: %v", path, err)
	}
	if !info.IsDir() || info.Mode().Perm() != want {
		t.Fatalf("expected %s to be a directory with mode %o, got %v", path, want, info.Mode())
	}
}
