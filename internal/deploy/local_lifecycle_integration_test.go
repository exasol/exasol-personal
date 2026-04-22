// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localruntime"
	localstate "github.com/exasol/exasol-personal/internal/localruntime/state"
	"github.com/exasol/exasol-personal/internal/presets"
)

func TestLocalLifecycleCommands_RunWithoutCloudCredentials(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("requires a POSIX shell helper process")
	}

	// Given
	restore := installLocalLifecycleTestHooks(t)
	defer restore()

	deploymentDir := t.TempDir()
	deployment := config.NewDeploymentDir(deploymentDir)
	initializeLocalLifecycleDeployment(t, deploymentDir)

	runtime := localruntime.New(deploymentDir)

	// When
	err := Deploy(context.Background(), deployment, false, TofuLockfileReadonly)

	// Then
	if err != nil {
		t.Fatalf("expected deploy to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentDir)

	info, err := config.ReadLocalDeploymentInfo(deploymentDir)
	if err != nil {
		t.Fatalf("expected local deployment info, got %v", err)
	}
	if info.Local == nil || info.Local.Host != localLoopbackHost {
		t.Fatalf("expected local loopback connection details, got %#v", info.Local)
	}

	instructions, err := os.ReadFile(filepath.Join(deploymentDir, config.ConnectionInstruction))
	if err != nil {
		t.Fatalf("expected connection instructions, got %v", err)
	}
	if len(instructions) == 0 ||
		!containsAll(
			string(instructions),
			"localhost",
			strconv.Itoa(info.Local.DBPort),
			strconv.Itoa(info.Local.UIPort),
		) {
		t.Fatalf("expected local connection instructions, got %q", string(instructions))
	}

	firstPID, err := runtime.ReadRunnerPID()
	if err != nil {
		t.Fatalf("expected local runner pid, got %v", err)
	}

	firstDBPort := info.Local.DBPort
	firstUIPort := info.Local.UIPort

	// When
	err = Stop(context.Background(), deployment, false)

	// Then
	if err != nil {
		t.Fatalf("expected stop to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateStopped](t, deploymentDir)

	if _, readErr := runtime.ReadRunnerPID(); !errors.Is(readErr, localruntime.ErrRuntimeNotRunning) {
		t.Fatalf("expected runner pid cleanup, got %v", readErr)
	}

	stoppedInfo, err := config.ReadLocalDeploymentInfo(deploymentDir)
	if err != nil {
		t.Fatalf("expected stopped local deployment info, got %v", err)
	}
	if stoppedInfo.ClusterState != localClusterStateStopped {
		t.Fatalf("expected stopped cluster state, got %q", stoppedInfo.ClusterState)
	}

	// When
	err = Start(context.Background(), deployment, false, 1)

	// Then
	if err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentDir)

	secondPID, err := runtime.ReadRunnerPID()
	if err != nil {
		t.Fatalf("expected restarted local runner pid, got %v", err)
	}
	if secondPID == firstPID {
		t.Fatalf("expected a new runner process after restart, got pid %d", secondPID)
	}

	restartedInfo, err := config.ReadLocalDeploymentInfo(deploymentDir)
	if err != nil {
		t.Fatalf("expected restarted local deployment info, got %v", err)
	}
	if restartedInfo.Local == nil {
		t.Fatal("expected local runtime info after restart")
	}
	if restartedInfo.Local.DBPort != firstDBPort || restartedInfo.Local.UIPort != firstUIPort {
		t.Fatalf(
			"expected local ports to be reused, got db=%d ui=%d",
			restartedInfo.Local.DBPort,
			restartedInfo.Local.UIPort,
		)
	}

	// When
	err = Destroy(context.Background(), deployment, false)

	// Then
	if err != nil {
		t.Fatalf("expected destroy to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateInitialized](t, deploymentDir)

	if _, statErr := os.Stat(runtime.Layout().RuntimeRoot()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected runtime root removal, got %v", statErr)
	}
	if _, infoErr := config.ReadLocalDeploymentInfo(deploymentDir); infoErr == nil {
		t.Fatal("expected local deployment info to be removed")
	}
	if _, statErr := os.Stat(filepath.Join(deploymentDir, config.ConnectionInstruction)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected connection instructions removal, got %v", statErr)
	}
}

func TestConcurrentLocalDeploymentsRemainIsolated(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("requires a POSIX shell helper process")
	}

	// Given
	restore := installLocalLifecycleTestHooks(t)
	defer restore()

	deploymentA := filepath.Join(t.TempDir(), "deployment-a")
	deploymentB := filepath.Join(t.TempDir(), "deployment-b")
	initializeLocalLifecycleDeployment(t, deploymentA)
	initializeLocalLifecycleDeployment(t, deploymentB)

	runtimeA := localruntime.New(deploymentA)
	runtimeB := localruntime.New(deploymentB)

	// When
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, deploymentDir := range []string{deploymentA, deploymentB} {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			errCh <- Deploy(
				context.Background(),
				config.NewDeploymentDir(dir),
				false,
				TofuLockfileReadonly,
			)
		}(deploymentDir)
	}
	wg.Wait()
	close(errCh)

	// Then
	for err := range errCh {
		if err != nil {
			t.Fatalf("expected concurrent deploy to succeed, got %v", err)
		}
	}
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentA)
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentB)

	infoA, err := config.ReadLocalDeploymentInfo(deploymentA)
	if err != nil {
		t.Fatalf("expected local deployment info for A, got %v", err)
	}
	infoB, err := config.ReadLocalDeploymentInfo(deploymentB)
	if err != nil {
		t.Fatalf("expected local deployment info for B, got %v", err)
	}
	if infoA.Local == nil || infoB.Local == nil {
		t.Fatalf("expected local runtime details, got A=%#v B=%#v", infoA.Local, infoB.Local)
	}
	if infoA.Local.RuntimeRoot == infoB.Local.RuntimeRoot {
		t.Fatalf("expected distinct runtime roots, got %q", infoA.Local.RuntimeRoot)
	}
	if infoA.Local.PIDFilePath == infoB.Local.PIDFilePath {
		t.Fatalf("expected distinct pid paths, got %q", infoA.Local.PIDFilePath)
	}

	pidA, err := runtimeA.ReadRunnerPID()
	if err != nil {
		t.Fatalf("expected runner pid for A, got %v", err)
	}
	pidB, err := runtimeB.ReadRunnerPID()
	if err != nil {
		t.Fatalf("expected runner pid for B, got %v", err)
	}
	if pidA == pidB {
		t.Fatalf("expected independent runner processes, got pid %d", pidA)
	}

	// When
	err = Stop(context.Background(), config.NewDeploymentDir(deploymentA), false)

	// Then
	if err != nil {
		t.Fatalf("expected stop for deployment A to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateStopped](t, deploymentA)
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentB)

	runningB, _, err := runtimeB.RunnerRunning()
	if err != nil {
		t.Fatalf("expected runner state for B, got %v", err)
	}
	if !runningB {
		t.Fatal("expected deployment B to remain running")
	}

	// When
	err = Destroy(context.Background(), config.NewDeploymentDir(deploymentA), false)

	// Then
	if err != nil {
		t.Fatalf("expected destroy for deployment A to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateInitialized](t, deploymentA)

	if _, infoErr := config.ReadLocalDeploymentInfo(deploymentB); infoErr != nil {
		t.Fatalf("expected deployment B info to remain available, got %v", infoErr)
	}
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentB)

	// Cleanup
	if err := Destroy(context.Background(), config.NewDeploymentDir(deploymentB), false); err != nil {
		t.Fatalf("expected cleanup destroy for deployment B to succeed, got %v", err)
	}
}

func installLocalLifecycleTestHooks(t *testing.T) func() {
	t.Helper()

	originalPlatformSupport := localRuntimePlatformSupported
	originalRunnerCommand := localRunnerCommand
	originalVerifyDatabaseConnection := verifyDatabaseConnectionFn

	var (
		processesMu sync.Mutex
		processes   []int
	)

	localRuntimePlatformSupported = func() bool { return true }
	verifyDatabaseConnectionFn = func(context.Context, config.DeploymentDir) error { return nil }
	localRunnerCommand = func(deploymentDir string, _ string) error {
		pid, err := startLocalLifecycleHelperProcess(deploymentDir)
		if err != nil {
			return err
		}

		processesMu.Lock()
		processes = append(processes, pid)
		processesMu.Unlock()

		return nil
	}

	return func() {
		localRuntimePlatformSupported = originalPlatformSupport
		localRunnerCommand = originalRunnerCommand
		verifyDatabaseConnectionFn = originalVerifyDatabaseConnection

		processesMu.Lock()
		defer processesMu.Unlock()
		for _, pid := range processes {
			process, err := os.FindProcess(pid)
			if err != nil {
				continue
			}

			_ = process.Kill()
		}
	}
}

func initializeLocalLifecycleDeployment(t *testing.T, deploymentDir string) {
	t.Helper()

	// Given
	if err := InitDeployment(
		context.Background(),
		PresetRef{Name: "local"},
		PresetRef{Name: presets.DefaultLocalInstallation},
		map[string]string{},
		map[string]string{},
		config.NewDeploymentDir(deploymentDir),
		false,
		"0.0.0",
	); err != nil {
		t.Fatalf("expected local init to succeed, got %v", err)
	}

	runtime := localruntime.New(deploymentDir)
	fixtureDir := t.TempDir()
	payloadPath := filepath.Join(fixtureDir, "exasol-nano-db-test-arm64.run")
	kernelPath := filepath.Join(fixtureDir, "vmlinux.container")
	initrdPath := filepath.Join(fixtureDir, "ubuntu-initrd.cpio.gz")
	for path, content := range map[string]string{
		payloadPath: "payload",
		kernelPath:  "kernel",
		initrdPath:  "initrd",
	} {
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatalf("expected fake runtime fixture %q, got %v", path, err)
		}
	}

	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 18563,
			"ui": 18443,
		},
		Payload: &localstate.PayloadRef{
			Version:      "1.2.3",
			Architecture: "arm64",
			CachePath:    payloadPath,
			Boot: &localstate.PayloadBootRef{
				KernelPath: kernelPath,
				InitrdPath: initrdPath,
			},
		},
	}); err != nil {
		t.Fatalf("expected local runtime state to be saved, got %v", err)
	}
}

func startLocalLifecycleHelperProcess(deploymentDir string) (int, error) {
	runtime := localruntime.New(deploymentDir)
	cmd := exec.Command(
		"sh",
		"-c",
		"(while [ ! -f \"$1\" ]; do sleep 0.025; done) >/dev/null 2>&1 & echo $!",
		"sh",
		runtime.Layout().StopRequestPath(),
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}
	if err := runtime.WriteRunnerPID(pid); err != nil {
		process, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = process.Kill()
		}
		return 0, err
	}

	return pid, nil
}

func assertWorkflowStateType[T any](t *testing.T, deploymentDir string) {
	t.Helper()

	state, err := config.ReadExasolPersonalState(config.NewDeploymentDir(deploymentDir))
	if err != nil {
		t.Fatalf("expected launcher state, got %v", err)
	}

	workflowState, err := state.GetWorkflowState()
	if err != nil {
		t.Fatalf("expected workflow state, got %v", err)
	}

	if _, ok := workflowState.(T); !ok {
		t.Fatalf("expected workflow state %T, got %T", *new(T), workflowState)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}

	return true
}
