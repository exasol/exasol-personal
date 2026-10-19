// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerInstallStart_StartsFreshDatabaseWithDockerArguments(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newDockerInstallFixture(t)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected fresh start to succeed, got %v", err)
	}
	assertCommandLog(t, fixture.logPath, []string{
		dockerListCommand,
		"<docker><load><-i><" + fixture.imagePath + ">",
		"<sync>",
		dockerListCommand,
		"<docker><run><-d><--name><" + testContainerName + ">" +
			"<--shm-size=512mb><--pids-limit=-1>" +
			"<--security-opt><systempaths=unconfined>" +
			"<--security-opt><seccomp=unconfined>" +
			"<--security-opt><apparmor=unconfined>" +
			"<--security-opt><label=disable>" +
			"<--restart><always><-p><127.0.0.1:28563:8563>" +
			"<-v><" + startConfig.DataDir + ":/exa:Z>" +
			"<" + testLoadedImage + "><init><params=maxConnectionsLicenseLimit=20>" +
			"<VERSION_CHECK_ENABLED=0>",
	})
}

func TestDockerInstallStart_RemovesStoppedContainerBeforeRun(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newDockerInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "existing"), "")

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	if len(commands) < 2 || commands[len(commands)-2] != "<docker><rm><--force><"+
		testContainerName+">" {
		t.Fatalf("expected the stopped container to be removed before run, got %q", commands)
	}
}

func TestDockerInstallStart_UsesLoadedImageIDWhenUntagged(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	const loadedImageID = "sha256:0123456789abcdef"
	install, startConfig, fixture := newDockerInstallFixture(t)
	writeTestFile(
		t,
		filepath.Join(fixture.scenarioDir, "load-output"),
		loadedImageIDPrefix+" "+loadedImageID+"\n",
	)

	// When
	err := install.Start(context.Background(), nil, nil, startConfig)
	// Then
	if err != nil {
		t.Fatalf("expected start with loaded image ID to succeed, got %v", err)
	}
	commands := readCommandLog(t, fixture.logPath)
	if runCommand := commands[len(commands)-1]; !strings.Contains(
		runCommand,
		"<"+loadedImageID+">",
	) {
		t.Fatalf("expected run command to use loaded image ID, got %q", runCommand)
	}
}

func TestDockerInstallStop_SkipsRemovalOfAbsentContainer(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, _, fixture := newDockerInstallFixture(t)

	// When
	err := install.Stop(context.Background(), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected stop of an absent container to succeed, got %v", err)
	}
	assertCommandLog(t, fixture.logPath, []string{dockerListCommand})
}

func TestDockerInstallImageExists_ReadsImageListing(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, _, fixture := newDockerInstallFixture(t)
	writeTestFile(t, filepath.Join(fixture.scenarioDir, "images"), "present:1\n")

	for image, want := range map[string]bool{"present:1": true, "absent:1": false} {
		// When
		exists, err := install.imageExists(context.Background(), nil, image)
		// Then
		if err != nil || exists != want {
			t.Fatalf("expected %s exists=%t, got %t (err=%v)", image, want, exists, err)
		}
	}
}

func TestDockerInstallClearDataDir_DeletesContentsInsideContainer(t *testing.T) {
	t.Parallel()
	skipPodmanInstallTestOnWindows(t)

	// Given
	install, startConfig, fixture := newDockerInstallFixture(t)

	// When
	err := install.ClearDataDir(context.Background(), nil, nil, startConfig.DataDir)
	// Then
	if err != nil {
		t.Fatalf("expected data cleanup to succeed, got %v", err)
	}
	assertCommandLog(t, fixture.logPath, []string{
		"<docker><run><--rm><--security-opt><label=disable>" +
			"<-v><" + startConfig.DataDir + ":/data><" + dataCleanupImage + ">" +
			"<find></data><-mindepth><1><-delete>",
	})
}

const dockerListCommand = "<docker><ps><--all><--filter><name=" + testContainerName +
	"><--format><{{.Names}}>"

func newDockerInstallFixture(t *testing.T) (*PodmanInstall, StartConfig, podmanInstallFixture) {
	t.Helper()

	podmanInstall, startConfig, fixture := newPodmanInstallFixture(t)
	scriptPath := filepath.Join(t.TempDir(), "fake-docker.sh")
	if err := os.WriteFile(
		scriptPath, []byte(fakeDockerScript), testExecutableFileMode,
	); err != nil {
		t.Fatalf("failed to write fake Docker executable: %v", err)
	}
	install := NewPodmanInstallWithEnvironment(
		podmanInstall.deployment,
		EngineDocker,
		NewDirectExecutionEnvironment(
			[]string{"/bin/sh", scriptPath, fixture.logPath, fixture.scenarioDir},
		),
		podmanInstall.resolveImage,
		podmanInstall.slcStagingDir,
		podmanInstall.slcStatusPath,
	)

	return install, startConfig, fixture
}

const fakeDockerScript = `#!/bin/sh
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
if [ "$1" != "docker" ]; then
  exit 90
fi
case "$2" in
  ps)
    if [ -f "$scenario_dir/running" ] || [ -f "$scenario_dir/existing" ]; then
      printf '%s\n' "${5#name=}"
    fi
    ;;
  container)
    if [ "$5" = "{{json .Mounts}}" ]; then
      printf '[{"Type":"bind","Destination":"/exa"}]\n'
    elif [ -f "$scenario_dir/running" ]; then
      printf 'true\n'
    else
      printf 'false\n'
    fi
    ;;
  images)
    if [ -f "$scenario_dir/images" ] && grep -Fxq "$4" "$scenario_dir/images"; then
      printf 'sha256:image\n'
    fi
    ;;
  load)
    if [ -f "$scenario_dir/load-output" ]; then
      cat "$scenario_dir/load-output"
    else
      printf 'Loaded image: docker.io/exasol/nano:test\n'
    fi
    ;;
  rm)
    rm -f "$scenario_dir/running" "$scenario_dir/existing"
    ;;
  run|info|logs)
    ;;
  *)
    printf 'unexpected fake Docker command: %s\n' "$2" >&2
    exit 91
    ;;
esac
`

func TestNanoRunArgs_MountsSLCsReadOnlyForDocker(t *testing.T) {
	t.Parallel()

	slc := SLCConfig{Image: "slc:1", Target: "/exa/slc/python"}
	for engine, want := range map[ContainerEngine]string{
		EngineDocker: "type=image,source=slc:1,destination=/exa/slc/python",
		EnginePodman: "type=image,source=slc:1,destination=/exa/slc/python,rw=true",
	} {
		args := nanoRunArgs(
			StartConfig{ContainerDBPort: 8563, DataDir: "/data"},
			nanoRunOptions{engine: engine, slcs: []SLCConfig{slc}},
		)
		if !strings.Contains(strings.Join(args, " "), "--mount "+want+" ") {
			t.Fatalf("expected %s mount %q, got %q", engine, want, args)
		}
	}
}
