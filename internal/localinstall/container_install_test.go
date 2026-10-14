// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
)

const (
	testAppleContainerDeploymentID = "apple-container-install-test"
	testAppleContainerName         = "exasol-db-" + testAppleContainerDeploymentID
	testAppleContainerLoadedImage  = "docker.io/exasol/nano@sha256:test"
)

func TestContainerRunArgs_FreshBootstrapIncludesInitAndPasswordFile(t *testing.T) {
	t.Parallel()

	startConfig := StartConfig{
		ContainerDBPort:     28563,
		ContainerDBBindHost: "127.0.0.1",
		DataDir:             "/data/exa",
		InitParams:          []string{"maxConnectionsLicenseLimit=20"},
		BootstrapDir:        "/data/bootstrap",
	}
	args := containerRunArgs(startConfig, testAppleContainerName, testAppleContainerLoadedImage, true, true)
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"run -d",
		"--name " + testAppleContainerName,
		"-p 127.0.0.1:28563:8563",
		"-v /data/exa:/exa",
		"-v /data/bootstrap:/run/secrets:ro",
		testAppleContainerLoadedImage + " init",
		"params=maxConnectionsLicenseLimit=20",
		"sys_password_file=/run/secrets/sys_password",
		"VERSION_CHECK_ENABLED=0",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected run args to contain %q, got %q", want, joined)
		}
	}
	if strings.Contains(joined, "--restart") {
		t.Fatalf("container run args must not include --restart, got %q", joined)
	}
	if strings.Contains(joined, "--security-opt") {
		t.Fatalf("container run args must not include --security-opt, got %q", joined)
	}
}

func TestContainerRunArgs_RestartOmitsInitParamsAndBootstrap(t *testing.T) {
	t.Parallel()

	startConfig := StartConfig{
		ContainerDBPort: 8563,
		DataDir:         "/data/exa",
		InitParams:      []string{"maxConnectionsLicenseLimit=20"},
	}
	args := containerRunArgs(startConfig, testAppleContainerName, testAppleContainerLoadedImage, false, false)
	joined := strings.Join(args, " ")

	for _, unwanted := range []string{"params=", "sys_password_file=", ":ro"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("restart run args must not contain %q, got %q", unwanted, joined)
		}
	}
}

func TestAppleContainerValidateStartConfig_RejectsScriptLanguageContainers(t *testing.T) {
	t.Parallel()

	install := &AppleContainerInstall{}
	startConfig := StartConfig{
		ContainerDBPort: 8563,
		DataDir:         "/data/exa",
		SLCs:            []SLCConfig{{Image: "slc/python", Target: "/some/target"}},
	}
	err := install.validateStartConfig(startConfig)
	if !errors.Is(err, ErrScriptLanguageContainersUnsupported) {
		t.Fatalf("expected ErrScriptLanguageContainersUnsupported, got %v", err)
	}
}

func TestAppleContainerValidateStartConfig_AcceptsConfigWithoutSLCs(t *testing.T) {
	t.Parallel()

	install := &AppleContainerInstall{}
	startConfig := StartConfig{
		ContainerDBPort: 8563,
		DataDir:         "/data/exa",
	}
	if err := install.validateStartConfig(startConfig); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAppleContainerInstallStart_StartsFreshDatabaseAndBootstraps(t *testing.T) {
	skipAppleContainerInstallTestOnWindows(t)
	t.Parallel()

	install, startConfig, fixture := newAppleContainerInstallFixture(t)
	withTestAppleContainerInitialPassword(&startConfig)

	if err := install.Start(context.Background(), nil, nil, startConfig); err != nil {
		t.Fatalf("expected Start to succeed, got %v", err)
	}
	log := readAppleContainerCommandLog(t, fixture.logPath)
	if len(log) != 4 {
		t.Fatalf("expected 4 commands, got %#v", log)
	}
	if log[0] != "<container><inspect><"+testAppleContainerName+">" {
		t.Errorf("unexpected inspect command %q", log[0])
	}
	// The repacked image path is a fresh staging file, not fixture.imagePath:
	// Start repacks the archive before loading it (see
	// repackWithTraversableDirectories) so only the directory and suffix
	// are asserted here.
	if !strings.Contains(log[1], "<container><image><load><--input></") ||
		!strings.HasSuffix(log[1], ".tar>") {
		t.Errorf("unexpected image load command %q", log[1])
	}
	if log[2] != "<sync>" {
		t.Errorf("unexpected sync command %q", log[2])
	}
	expectedRun := "<container><run><-d><--name><" + testAppleContainerName + "><--shm-size=512M><--ulimit><nproc=unlimited>" +
		"<-p><127.0.0.1:28563:8563><-v><" + startConfig.DataDir + ":/exa>" +
		"<-v><" + startConfig.BootstrapDir + ":/run/secrets:ro>" +
		"<" + testAppleContainerLoadedImage + "><init>" +
		"<params=maxConnectionsLicenseLimit=20>" +
		"<sys_password_file=/run/secrets/sys_password><VERSION_CHECK_ENABLED=0>"
	if log[3] != expectedRun {
		t.Errorf("expected run command %q, got %q", expectedRun, log[3])
	}
	passwordPath := filepath.Join(startConfig.BootstrapDir, nanoBootstrapPasswordFile)
	data, err := os.ReadFile(passwordPath) //nolint:gosec // test-owned fixture path
	if err != nil {
		t.Fatalf("expected bootstrap password file to exist, got %v", err)
	}
	if string(data) != testAppleContainerInitialPassword {
		t.Fatalf("expected bootstrap password %q, got %q", testAppleContainerInitialPassword, data)
	}
}

func TestAppleContainerInstallStart_ReturnsWhenAlreadyRunning(t *testing.T) {
	skipAppleContainerInstallTestOnWindows(t)
	t.Parallel()

	install, startConfig, fixture := newAppleContainerInstallFixture(t)
	markAppleContainerRunning(t, fixture.scenarioDir)

	if err := install.Start(context.Background(), nil, nil, startConfig); err != nil {
		t.Fatalf("expected Start to succeed, got %v", err)
	}
	assertAppleContainerCommandLog(t, fixture.logPath, []string{
		"<container><inspect><" + testAppleContainerName + ">",
	})
}

func TestAppleContainerInstallStart_RemovesStaleStoppedContainerBeforeRecreating(t *testing.T) {
	skipAppleContainerInstallTestOnWindows(t)
	t.Parallel()

	install, startConfig, fixture := newAppleContainerInstallFixture(t)
	markAppleContainerExisting(t, fixture.scenarioDir)

	if err := install.Start(context.Background(), nil, nil, startConfig); err != nil {
		t.Fatalf("expected Start to succeed, got %v", err)
	}
	log := readAppleContainerCommandLog(t, fixture.logPath)
	if len(log) < 2 || log[1] != "<container><delete><--force><"+testAppleContainerName+">" {
		t.Fatalf("expected the second command to delete the stale container, got %#v", log)
	}
}

func TestAppleContainerInstallStop_RemovesContainerIdempotently(t *testing.T) {
	skipAppleContainerInstallTestOnWindows(t)
	t.Parallel()

	install, _, fixture := newAppleContainerInstallFixture(t)

	if err := install.Stop(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected Stop on an absent container to be a no-op, got %v", err)
	}
	assertAppleContainerCommandLog(t, fixture.logPath, []string{
		"<container><inspect><" + testAppleContainerName + ">",
	})

	markAppleContainerExisting(t, fixture.scenarioDir)
	if err := install.Stop(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected Stop to succeed, got %v", err)
	}
}

func TestAppleContainerInstallStatus_ReportsRunningState(t *testing.T) {
	skipAppleContainerInstallTestOnWindows(t)
	t.Parallel()

	install, _, fixture := newAppleContainerInstallFixture(t)

	status, err := install.Status(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("expected Status to succeed, got %v", err)
	}
	if status.Running {
		t.Fatalf("expected a fresh fixture to report not running")
	}

	markAppleContainerRunning(t, fixture.scenarioDir)
	status, err = install.Status(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("expected Status to succeed, got %v", err)
	}
	if !status.Running {
		t.Fatalf("expected status to report running")
	}
}

func withTestAppleContainerInitialPassword(startConfig *StartConfig) {
	startConfig.InitialPassword = func(context.Context) (string, error) {
		return testAppleContainerInitialPassword, nil
	}
}

const testAppleContainerInitialPassword = "Initial1Password"

type appleContainerInstallFixture struct {
	logPath     string
	scenarioDir string
	imagePath   string
}

func newAppleContainerInstallFixture(
	t *testing.T,
) (*AppleContainerInstall, StartConfig, appleContainerInstallFixture) {
	t.Helper()

	root := t.TempDir()
	deployment := config.NewDeploymentDir(filepath.Join(root, "deployment"))
	if err := os.MkdirAll(deployment.Root(), 0o750); err != nil {
		t.Fatalf("failed to create deployment directory: %v", err)
	}
	state := &config.ExasolPersonalState{DeploymentId: testAppleContainerDeploymentID}
	if err := state.SetWorkflowStateAndWrite(
		&config.WorkflowStateInitialized{},
		deployment,
	); err != nil {
		t.Fatalf("failed to write deployment state: %v", err)
	}

	scenarioDir := filepath.Join(root, "scenario")
	if err := os.MkdirAll(scenarioDir, 0o750); err != nil {
		t.Fatalf("failed to create fake container scenario: %v", err)
	}
	scriptPath := filepath.Join(root, "fake-container.sh")
	if err := os.WriteFile(scriptPath, []byte(fakeContainerScript), 0o700); err != nil {
		t.Fatalf("failed to write fake container executable: %v", err)
	}
	logPath := filepath.Join(root, "container.log")
	imagePath := filepath.Join(root, "nano-image.tar")
	writeFakeOCIArchive(t, imagePath)
	manager := runtimeartifacts.NewResourceManagerForPlatform(
		runtimeartifacts.ResourceSpec{
			exasolNanoImageResourceID: {
				Artifact: map[string]runtimeartifacts.ArtifactSpec{
					"any": {URL: imagePath},
				},
			},
		},
		filepath.Join(root, "cache"),
		runtime.GOOS,
		runtime.GOARCH,
	)

	environment := NewDirectExecutionEnvironment(
		[]string{"/bin/sh", scriptPath, logPath, scenarioDir},
	)
	resolveImage := func(ctx context.Context) (RuntimePath, error) {
		path, err := ResolveNanoImage(ctx, manager)
		if err != nil {
			return RuntimePath{}, err
		}

		return IdentityRuntimePath(path), nil
	}
	install := NewAppleContainerInstallWithEnvironment(
		deployment,
		environment,
		resolveImage,
		IdentityRuntimePath(filepath.Join(root, "slc-packages")),
		IdentityRuntimePath(filepath.Join(root, "slc-status.json")),
	)
	startConfig := StartConfig{
		ContainerDBPort:     28563,
		ContainerDBBindHost: "127.0.0.1",
		DataDir:             filepath.Join(root, "runtime", "exa"),
		InitParams:          []string{"maxConnectionsLicenseLimit=20"},
		BootstrapDir:        filepath.Join(root, "bootstrap"),
	}

	return install, startConfig, appleContainerInstallFixture{
		logPath:     logPath,
		scenarioDir: scenarioDir,
		imagePath:   imagePath,
	}
}

func markAppleContainerRunning(t *testing.T, scenarioDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(scenarioDir, "running"), nil, 0o600); err != nil {
		t.Fatalf("failed to mark fake container running: %v", err)
	}
}

func markAppleContainerExisting(t *testing.T, scenarioDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(scenarioDir, "existing"), nil, 0o600); err != nil {
		t.Fatalf("failed to mark fake container existing: %v", err)
	}
}

func readAppleContainerCommandLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test-owned fixture path
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("failed to read fake container command log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func assertAppleContainerCommandLog(t *testing.T, path string, expected []string) {
	t.Helper()
	actual := readAppleContainerCommandLog(t, path)
	if len(actual) != len(expected) {
		t.Fatalf("expected command log %#v, got %#v", expected, actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected command %d to be %q, got %q", index, expected[index], actual[index])
		}
	}
}

// writeFakeOCIArchive writes a minimal OCI-layout tar whose directory entry
// carries mode 0600, the way the pinned Nano image archive's docker://
// source actually produces it (see repackWithTraversableDirectories). This
// is a regression fixture for that real, observed failure: without the
// repack step, `container image load` (and the standard `tar` command)
// cannot traverse a directory lacking the owner execute bit.
func writeFakeOCIArchive(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path) //nolint:gosec // test-owned fixture path
	if err != nil {
		t.Fatalf("failed to create fake image archive: %v", err)
	}
	defer func() { _ = file.Close() }()

	writer := tar.NewWriter(file)
	if err := writer.WriteHeader(&tar.Header{
		Name:     "blobs/",
		Typeflag: tar.TypeDir,
		Mode:     0o600,
	}); err != nil {
		t.Fatalf("failed to write fake image archive directory header: %v", err)
	}
	if err := writer.WriteHeader(&tar.Header{
		Name:     "oci-layout",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len("fake-oci-layout")),
	}); err != nil {
		t.Fatalf("failed to write fake image archive file header: %v", err)
	}
	if _, err := writer.Write([]byte("fake-oci-layout")); err != nil {
		t.Fatalf("failed to write fake image archive file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to finalize fake image archive: %v", err)
	}
}

func skipAppleContainerInstallTestOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake container executable is a POSIX shell script")
	}
}

//nolint:dupword // Repeated shell terminators are required by this fixture.
const fakeContainerScript = `#!/bin/sh
set -eu

log_path=$1
scenario_dir=$2
shift 2

for argument in "$@"; do
  printf '<%s>' "$argument" >> "$log_path"
done
printf '\n' >> "$log_path"

if [ "$1" = "sync" ]; then
  exit 0
fi
if [ "$1" != "container" ]; then
  exit 90
fi
command=$2

case "$command" in
  inspect)
    name=$3
    if [ -f "$scenario_dir/running" ]; then
      printf '[{"status":{"state":"running"}}]\n'
    elif [ -f "$scenario_dir/existing" ]; then
      printf '[{"status":{"state":"stopped"}}]\n'
    else
      printf 'Error: container not found: %s\n' "$name" >&2
      exit 1
    fi
    ;;
  image)
    if [ -f "$scenario_dir/load-output" ]; then
      cat "$scenario_dir/load-output"
    else
      printf '` + testAppleContainerLoadedImage + `\n'
    fi
    ;;
  run)
    touch "$scenario_dir/running"
    ;;
  delete)
    rm -f "$scenario_dir/running" "$scenario_dir/existing"
    ;;
  system|list|logs)
    :
    ;;
  *)
    printf 'unexpected fake container operation: %s\n' "$command" >&2
    exit 92
    ;;
esac
`
