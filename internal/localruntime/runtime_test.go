// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/runtimeartifacts"
	"golang.org/x/crypto/ssh"
)

const windowsGOOS = "windows"

// runnerZipEntryName matches resources.yaml's resource_path for
// exasol-local-runner, so these tests exercise the same extract +
// resource_path shape production resolves through.
const runnerZipEntryName = "launcher"

// newTestManagerForRunner builds a Manager whose "exasol-local-runner"
// resource resolves through the same extract: true / resource_path shape the
// real resources.yaml entry uses: scriptContent is packed into a minimal,
// single-entry zip (mirroring the real release archive), and FileSource's
// local-path redirect + the existing ZipExtractor unpack it, preserving the
// executable mode recorded in the zip entry.
func newTestManagerForRunner(t *testing.T, scriptContent []byte) *runtimeartifacts.Manager {
	t.Helper()

	zipPath := writeRunnerZip(t, scriptContent)
	spec := runtimeartifacts.ResourceSpec{
		exasolLocalRunnerResourceID: {
			Extract: true,
			Artifact: map[string]runtimeartifacts.ArtifactSpec{
				"any": {URL: zipPath, ResourcePath: runnerZipEntryName},
			},
		},
	}

	return runtimeartifacts.NewResourceManagerForPlatform(
		spec, t.TempDir(), runtime.GOOS, runtime.GOARCH,
	)
}

func writeRunnerZip(t *testing.T, scriptContent []byte) string {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "runner.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create runner zip fixture: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: runnerZipEntryName, Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create runner zip entry: %v", err)
	}
	if _, err := entry.Write(scriptContent); err != nil {
		t.Fatalf("failed to write runner zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close runner zip fixture: %v", err)
	}

	return zipPath
}

func TestReadRunnerState_ParsesForwardedPorts(t *testing.T) {
	t.Parallel()

	// Given
	statePath := filepath.Join(t.TempDir(), "vm-state.json")
	writeRunnerStateFile(t, statePath, map[string]any{
		"vm_name": "exasol-local-vm",
		"vm_ip":   "192.168.64.2",
		"ports": map[string]any{
			"ssh": 20022,
			"db":  28563,
			"ui":  28443,
		},
	})

	// When
	state, err := readRunnerState(statePath)
	// Then
	if err != nil {
		t.Fatalf("expected state to parse, got %v", err)
	}
	if state.Ports.SSH != 20022 || state.Ports.DB != 28563 || state.Ports.UI != 28443 {
		t.Fatalf("unexpected ports: %#v", state.Ports)
	}
}

func TestReadRunnerState_AcceptsMissingUIPort(t *testing.T) {
	t.Parallel()

	// Given
	statePath := filepath.Join(t.TempDir(), "vm-state.json")
	writeRunnerStateFile(t, statePath, map[string]any{
		"vm_name": "exasol-local-vm",
		"vm_ip":   "192.168.64.2",
		"ports": map[string]any{
			"ssh": 20022,
			"db":  28563,
			"ui":  0,
		},
	})

	// When
	state, err := readRunnerState(statePath)
	// Then
	if err != nil {
		t.Fatalf("expected state to parse with no UI port, got %v", err)
	}
	if state.Ports.SSH != 20022 || state.Ports.DB != 28563 || state.Ports.UI != 0 {
		t.Fatalf("unexpected ports: %#v", state.Ports)
	}
}

func TestReadRunnerState_RejectsInvalidPorts(t *testing.T) {
	t.Parallel()

	// Given
	statePath := filepath.Join(t.TempDir(), "vm-state.json")
	writeRunnerStateFile(t, statePath, map[string]any{
		"ports": map[string]any{
			"ssh": 0,
			"db":  28563,
			"ui":  28443,
		},
	})

	// When
	_, err := readRunnerState(statePath)

	// Then
	if err == nil {
		t.Fatal("expected invalid port error, got nil")
	}
}

func TestDestroy_RemovesLocalRuntime(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	paths := NewPaths(deployment)
	if err := os.MkdirAll(paths.Root, 0o750); err != nil {
		t.Fatalf("failed to create local runtime root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Root, "disk.img"), []byte("x"), 0o600); err != nil {
		t.Fatalf("failed to write test runtime file: %v", err)
	}

	// When: paths.VMDir was never created, so Destroy never needs to resolve
	// a runner, and a nil manager is safe here.
	err := New(deployment, nil).Destroy(context.Background(), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected destroy cleanup to succeed, got %v", err)
	}
	if _, statErr := os.Stat(paths.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, got stat error %v", paths.Root, statErr)
	}
}

// TestResolveRunnerPath_OverrideEnvBypassesManager uses a nil manager, so
// resolving through it would panic on a nil dereference -- proving the
// override truly bypasses the Manager rather than merely taking priority
// over some registered value.
func TestResolveRunnerPath_OverrideEnvBypassesManager(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	runnerPath := writeRunnerScript(t, "1.0.0")
	t.Setenv(runnerOverridePathEnv, runnerPath)

	// When
	resolved, err := localRuntime.resolveRunnerPath(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected override to resolve without a manager, got %v", err)
	}
	if resolved != runnerPath {
		t.Fatalf("expected resolved path %q, got %q", runnerPath, resolved)
	}
}

func TestPrepare_ResolvesRunnerAndRunsInitWithSSHKey(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	runnerScript := `#!/bin/sh
set -eu
case "$1" in
  version)
    printf 'v1.0.0\n'
    ;;
  init)
    if [ "$2" != "--ssh-key" ] || [ ! -f "$3" ]; then
      echo "expected init --ssh-key <private-key>, got: $*" >&2
      exit 4
    fi
    printf '%s' "$3" > init-key
    mkdir -p vm
    ;;
  *)
    echo "unexpected command: $1" >&2
    exit 2
    ;;
esac
`
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime := New(deployment, manager)

	// When
	err := localRuntime.Prepare(context.Background(), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected prepare to succeed, got %v", err)
	}
	keyPath, err := os.ReadFile(filepath.Join(localRuntime.paths.WorkDir, "init-key"))
	if err != nil {
		t.Fatalf("expected runner init key marker, got %v", err)
	}
	if string(keyPath) != localRuntime.paths.PrivateKeyPath {
		t.Fatalf("expected init key %q, got %q", localRuntime.paths.PrivateKeyPath, string(keyPath))
	}
	markerVersion, err := readRunnerVersionMarker(localRuntime.paths.RunnerVersionMarkerPath)
	if err != nil {
		t.Fatalf("expected a version marker to be recorded, got %v", err)
	}
	if markerVersion.String() != "1.0.0" {
		t.Fatalf("expected recorded version 1.0.0, got %s", markerVersion.String())
	}
}

func TestPrepare_SkipsInitWhenVMAlreadyInitialized(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.VMDir, 0o750); err != nil {
		t.Fatalf("failed to create local VM directory: %v", err)
	}
	runnerScript := `#!/bin/sh
case "$1" in
  version)
    printf 'v1.0.0\n'
    ;;
  *)
    echo "unexpected command: $1 (init should have been skipped)" >&2
    exit 2
    ;;
esac
`
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime = New(deployment, manager)

	// When
	err := localRuntime.Prepare(context.Background(), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected prepare to skip init and succeed, got %v", err)
	}
}

func TestEnsureSSHKey_PreservesExistingPrivateKey(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	existingKey := generateOpenSSHPrivateKey(t)
	if err := os.MkdirAll(filepath.Dir(localRuntime.paths.PrivateKeyPath), 0o750); err != nil {
		t.Fatalf("failed to create local key directory: %v", err)
	}
	if err := os.WriteFile(localRuntime.paths.PrivateKeyPath, existingKey, 0o600); err != nil {
		t.Fatalf("failed to write existing private key: %v", err)
	}

	// When
	if err := localRuntime.ensureSSHKey(); err != nil {
		t.Fatalf("expected SSH key setup to succeed, got %v", err)
	}
	if err := localRuntime.ensureSSHKey(); err != nil {
		t.Fatalf("expected repeated SSH key setup to succeed, got %v", err)
	}

	// Then
	data, err := os.ReadFile(localRuntime.paths.PrivateKeyPath)
	if err != nil {
		t.Fatalf("expected private key to be readable, got %v", err)
	}
	if string(data) != string(existingKey) {
		t.Fatalf("expected private key to be preserved, got %q", string(data))
	}
}

func TestEnsureSSHKey_GeneratesEd25519Key(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)

	// When
	if err := localRuntime.ensureSSHKey(); err != nil {
		t.Fatalf("expected SSH key setup to succeed, got %v", err)
	}

	// Then
	privateKey, err := os.ReadFile(localRuntime.paths.PrivateKeyPath)
	if err != nil {
		t.Fatalf("expected generated SSH private key to be readable, got %v", err)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		t.Fatalf("expected generated SSH private key to parse, got %v", err)
	}
	if signer.PublicKey().Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("expected ED25519 SSH key, got %q", signer.PublicKey().Type())
	}
	block, _ := pem.Decode(privateKey)
	if block == nil || block.Type != openSSHKeyPEMType {
		t.Fatalf("expected OpenSSH private key PEM, got %#v", block)
	}

	if _, err := os.Stat(filepath.Join(localRuntime.paths.WorkDir, "vm-shared")); err == nil {
		t.Fatal("expected SSH key setup not to create managed share")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to inspect managed share: %v", err)
	}
}

func TestEnsureSSHKey_ReplacesLegacyPKCS8PrivateKey(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	legacyKey := generatePKCS8PrivateKey(t)
	if err := os.MkdirAll(filepath.Dir(localRuntime.paths.PrivateKeyPath), 0o750); err != nil {
		t.Fatalf("failed to create local key directory: %v", err)
	}
	if err := os.WriteFile(localRuntime.paths.PrivateKeyPath, legacyKey, 0o600); err != nil {
		t.Fatalf("failed to write legacy private key: %v", err)
	}

	// When
	if err := localRuntime.ensureSSHKey(); err != nil {
		t.Fatalf("expected SSH key setup to succeed, got %v", err)
	}

	// Then
	privateKey, err := os.ReadFile(localRuntime.paths.PrivateKeyPath)
	if err != nil {
		t.Fatalf("expected generated SSH private key to be readable, got %v", err)
	}
	if string(privateKey) == string(legacyKey) {
		t.Fatal("expected legacy private key to be replaced")
	}
	block, _ := pem.Decode(privateKey)
	if block == nil || block.Type != openSSHKeyPEMType {
		t.Fatalf("expected replacement key in OpenSSH format, got %#v", block)
	}
	if _, err := ssh.ParsePrivateKey(privateKey); err != nil {
		t.Fatalf("expected replacement key to parse, got %v", err)
	}
}

func generateOpenSSHPrivateKey(t *testing.T) []byte {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test SSH key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		t.Fatalf("failed to marshal test SSH key: %v", err)
	}

	return pem.EncodeToMemory(block)
}

func generatePKCS8PrivateKey(t *testing.T) []byte {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test SSH key: %v", err)
	}
	data, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("failed to marshal test PKCS8 key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data})
}

func TestStop_InvokesResolvedRunnerStop(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}

	markerPath := filepath.Join(localRuntime.paths.WorkDir, "stop-called")
	runnerScript := "#!/bin/sh\nprintf '%s %s\\n' \"$0\" \"$*\" > stop-called\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime = New(deployment, manager)

	// When
	err := localRuntime.Stop(context.Background(), nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected runner stop to succeed, got %v", err)
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("expected runner stop marker, got %v", err)
	}
	markerText := string(marker)
	if !strings.Contains(markerText, " stop") {
		t.Fatalf("expected stop argument to be passed, got %q", markerText)
	}
	if !strings.HasSuffix(
		strings.Fields(markerText)[0],
		string(filepath.Separator)+runnerZipEntryName,
	) {
		t.Fatalf("expected stop to run through the resolved, extracted runner, got %q", markerText)
	}
}

func TestRunCommand_InvokesResolvedRunnerWithArgs(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}
	runnerScript := "#!/bin/sh\nprintf '%s\\n' \"$*\"\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime = New(deployment, manager)
	var out bytes.Buffer

	// When
	err := localRuntime.RunCommand(
		context.Background(),
		[]string{"start", "--ports", "auto"},
		&out,
		nil,
	)
	// Then
	if err != nil {
		t.Fatalf("expected RunCommand to succeed, got %v", err)
	}
	if strings.TrimSpace(out.String()) != "start --ports auto" {
		t.Fatalf("expected args to be passed through to the resolved runner, got %q", out.String())
	}
}

func TestDestroy_StopsRunningVMBeforeRemoving(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	paths := NewPaths(deployment)
	if err := os.MkdirAll(paths.VMDir, 0o750); err != nil {
		t.Fatalf("failed to create local VM directory: %v", err)
	}
	runnerScript := "#!/bin/sh\nprintf 'stop-invoked %s\\n' \"$*\"\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime := New(deployment, manager)
	var out bytes.Buffer

	// When
	err := localRuntime.Destroy(context.Background(), &out, nil)
	// Then
	if err != nil {
		t.Fatalf("expected destroy to succeed, got %v", err)
	}
	if !strings.Contains(out.String(), "stop-invoked stop") {
		t.Fatalf(
			"expected destroy to resolve and stop the running VM before removing it, got output %q",
			out.String(),
		)
	}
	if _, statErr := os.Stat(paths.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, got stat error %v", paths.Root, statErr)
	}
}

func TestHealthCheck_ParsesPortStates(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}

	runnerScript := `#!/bin/sh
echo '{"ports":{"ssh":{"state":"reachable"},"db":{"state":"blocked"}}}'
`
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime = New(deployment, manager)

	// When
	result, err := localRuntime.HealthCheck(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected health-check to succeed, got %v", err)
	}
	if result.Ports["ssh"].State != PortStateReachable {
		t.Fatalf("ssh state = %q, want %q", result.Ports["ssh"].State, PortStateReachable)
	}
	if result.Ports["db"].State != PortStateBlocked {
		t.Fatalf("db state = %q, want %q", result.Ports["db"].State, PortStateBlocked)
	}
}

func TestHealthCheck_ReturnsErrorOnRunnerFailure(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given: an old runner that does not understand "health-check" yet.
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}

	runnerScript := "#!/bin/sh\necho 'Unknown command: health-check' >&2\nexit 1\n"
	manager := newTestManagerForRunner(t, []byte(runnerScript))
	localRuntime = New(deployment, manager)

	// When
	_, err := localRuntime.HealthCheck(context.Background())
	// Then
	if err == nil {
		t.Fatal("expected health-check against an unsupporting runner to fail")
	}
}

func TestWaitForDaemonExit_IgnoresMissingPIDFile(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}

	// When
	err := localRuntime.waitForDaemonExit(context.Background())
	// Then
	if err != nil {
		t.Fatalf("expected missing PID file to be treated as stopped, got %v", err)
	}
}

func TestWaitForDaemonExit_RejectsStillRunningPID(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("process signal checks are POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := New(deployment, nil)
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(localRuntime.paths.WorkDir, vmPIDFileName),
		[]byte(strconv.Itoa(os.Getpid())),
		0o600,
	); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// When
	err := localRuntime.waitForDaemonExit(ctx)

	// Then
	if err == nil {
		t.Fatal("expected still-running PID to prevent stop completion")
	}
}

func writeRunnerStateFile(t *testing.T, path string, state map[string]any) {
	t.Helper()

	writeJSONFile(t, path, state)
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("failed to create parent directory for %s: %v", path, err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal JSON value: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write JSON file %s: %v", path, err)
	}
}

func writeExecutableTestFile(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.WriteFile(path, content, privateFileMode); err != nil {
		t.Fatalf("failed to write executable test file %s: %v", path, err)
	}
	if err := os.Chmod(path, executableFileMode); err != nil {
		t.Fatalf("failed to mark executable test file %s executable: %v", path, err)
	}
}
