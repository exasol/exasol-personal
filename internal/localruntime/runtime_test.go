// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/exasol/exasol-personal/assets/localruntimebin"
	"github.com/exasol/exasol-personal/internal/config"
	"golang.org/x/crypto/ssh"
)

const windowsGOOS = "windows"

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

	// When
	err := Destroy(context.Background(), deployment, nil, nil)
	// Then
	if err != nil {
		t.Fatalf("expected destroy cleanup to succeed, got %v", err)
	}
	if _, statErr := os.Stat(paths.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected %s to be removed, got stat error %v", paths.Root, statErr)
	}
}

func TestEnsureRunnerExecutable_DoesNotOverwriteExistingRunner(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := newRuntime(deployment, Config{})
	existingContent := []byte("#!/bin/sh\necho existing runner\n")
	if err := os.MkdirAll(filepath.Dir(localRuntime.paths.RunnerPath), 0o750); err != nil {
		t.Fatalf("failed to create runner directory: %v", err)
	}
	writeExecutableTestFile(t, localRuntime.paths.RunnerPath, existingContent)

	// When
	err := localRuntime.ensureRunnerExecutable()
	// Then
	if err != nil {
		t.Fatalf("expected existing runner to be accepted, got %v", err)
	}
	data, err := os.ReadFile(localRuntime.paths.RunnerPath)
	if err != nil {
		t.Fatalf("expected existing runner to be readable, got %v", err)
	}
	if string(data) != string(existingContent) {
		t.Fatalf("expected existing runner not to be overwritten, got %q", string(data))
	}
}

func TestEnsureSSHKey_PreservesExistingAuthorizedKeys(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := newRuntime(deployment, Config{})
	runnerKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKAVO4cmQ+ROEQB2/IAaUVt6s4vTz5lyMgRxKAxlwPqJ"
	authorizedKeysPath := filepath.Join(localRuntime.paths.ShareDir, authorizedKeysFile)
	if err := os.MkdirAll(localRuntime.paths.ShareDir, 0o750); err != nil {
		t.Fatalf("failed to create local managed share: %v", err)
	}
	if err := os.WriteFile(authorizedKeysPath, []byte(runnerKey+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write existing authorized keys: %v", err)
	}

	// When
	if err := localRuntime.ensureSSHKey(); err != nil {
		t.Fatalf("expected SSH key setup to succeed, got %v", err)
	}
	if err := localRuntime.ensureSSHKey(); err != nil {
		t.Fatalf("expected repeated SSH key setup to succeed, got %v", err)
	}

	// Then
	data, err := os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("expected authorized keys to be readable, got %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf(
			"expected runner key plus launcher key, got %d keys:\n%s",
			len(lines),
			string(data),
		)
	}
	if lines[0] != runnerKey {
		t.Fatalf("expected runner key to be preserved first, got %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) == "" {
		t.Fatalf("expected launcher key to be appended, got %q", lines[1])
	}
}

func TestEnsureSSHKey_GeneratesEd25519Key(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := newRuntime(deployment, Config{})

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

	authorizedKeysPath := filepath.Join(localRuntime.paths.ShareDir, authorizedKeysFile)
	authorizedKeys, err := os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("expected generated authorized keys to be readable, got %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(authorizedKeys)), ssh.KeyAlgoED25519+" ") {
		t.Fatalf("expected ED25519 authorized key, got %q", string(authorizedKeys))
	}
}

func TestStop_InvokesOriginalRunnerStop(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("fake local runner script is POSIX-only")
	}

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := newRuntime(deployment, Config{})
	if err := os.MkdirAll(localRuntime.paths.WorkDir, 0o750); err != nil {
		t.Fatalf("failed to create local runtime directory: %v", err)
	}

	markerPath := filepath.Join(localRuntime.paths.WorkDir, "stop-called")
	runnerScript := "#!/bin/sh\nprintf '%s %s\\n' \"$0\" \"$*\" > stop-called\n"
	writeExecutableTestFile(t, localRuntime.paths.RunnerPath, []byte(runnerScript))

	// When
	err := Stop(context.Background(), deployment, nil, nil)
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
	if !strings.Contains(markerText, RunnerFileName) {
		t.Fatalf("expected stop to run through the original runner, got %q", markerText)
	}
}

func TestWaitForDaemonExit_IgnoresMissingPIDFile(t *testing.T) {
	t.Parallel()

	// Given
	deployment := config.NewDeploymentDir(t.TempDir())
	localRuntime := newRuntime(deployment, Config{})
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
	localRuntime := newRuntime(deployment, Config{})
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

func TestWriteEmbeddedRunner_WritesBundledRunner(t *testing.T) {
	t.Parallel()

	if !localruntimebin.RunnerBinaryAvailable {
		t.Skip("embedded local runner is only available for macOS Apple Silicon builds")
	}

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)

	// When
	err := writeEmbeddedRunner(targetPath)
	// Then
	if err != nil {
		t.Fatalf("expected embedded runner to be written, got %v", err)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected embedded runner to be readable, got %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected embedded runner to be non-empty")
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("expected embedded runner to exist, got %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("expected embedded runner mode 0700, got %o", info.Mode().Perm())
	}
}

func TestWriteEmbeddedRunner_DoesNotOverwriteExistingRunner(t *testing.T) {
	t.Parallel()

	// Given
	targetPath := filepath.Join(t.TempDir(), RunnerFileName)
	existingContent := []byte("#!/bin/sh\necho existing runner\n")
	writeExecutableTestFile(t, targetPath, existingContent)

	// When
	err := writeEmbeddedRunner(targetPath)
	// Then
	if err != nil {
		t.Fatalf("expected existing runner to be accepted, got %v", err)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected existing runner to be readable, got %v", err)
	}
	if string(data) != string(existingContent) {
		t.Fatalf("expected embedded runner not to overwrite existing runner, got %q", string(data))
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
