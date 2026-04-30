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

//nolint:paralleltest // modifies package-level hooks for the local runtime.
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

	info := mustReadLocalDeploymentInfo(t, deploymentDir)
	if info.Runtime.Host != localLoopbackHost {
		t.Fatalf("expected local loopback connection details, got %#v", info.Runtime)
	}

	instructions, err := os.ReadFile(filepath.Join(deploymentDir, config.ConnectionInstruction))
	if err != nil {
		t.Fatalf("expected connection instructions, got %v", err)
	}
	if len(instructions) == 0 ||
		!containsAll(
			string(instructions),
			"localhost",
			strconv.Itoa(info.Runtime.DBPort),
			strconv.Itoa(info.Runtime.UIPort),
			`Login with username "`+localDefaultDatabaseUser+`"`,
		) {
		t.Fatalf("expected local connection instructions, got %q", string(instructions))
	}
	if strings.Contains(string(instructions), `Login with username "admin"`) {
		t.Fatalf(
			"expected local connection instructions to avoid admin username, got %q",
			string(instructions),
		)
	}

	firstPID, err := runtime.ReadRunnerPID()
	if err != nil {
		t.Fatalf("expected local runner pid, got %v", err)
	}

	firstDBPort := info.Runtime.DBPort
	firstUIPort := info.Runtime.UIPort

	// When
	err = Stop(context.Background(), deployment, false)
	// Then
	if err != nil {
		t.Fatalf("expected stop to succeed, got %v", err)
	}
	assertWorkflowStateType[*config.WorkflowStateStopped](t, deploymentDir)

	if _, readErr := runtime.ReadRunnerPID(); !errors.Is(
		readErr,
		localruntime.ErrRuntimeNotRunning,
	) {
		t.Fatalf("expected runner pid cleanup, got %v", readErr)
	}

	stoppedInfo := mustReadLocalDeploymentInfo(t, deploymentDir)
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

	restartedInfo := mustReadLocalDeploymentInfo(t, deploymentDir)
	if restartedInfo.Runtime.DBPort != firstDBPort || restartedInfo.Runtime.UIPort != firstUIPort {
		t.Fatalf(
			"expected local ports to be reused, got db=%d ui=%d",
			restartedInfo.Runtime.DBPort,
			restartedInfo.Runtime.UIPort,
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
	if _, infoErr := config.ReadDeploymentInfo(
		config.NewDeploymentDir(deploymentDir),
	); infoErr == nil {
		t.Fatal("expected local deployment info to be removed")
	}
	if _, statErr := os.Stat(
		filepath.Join(deploymentDir, config.ConnectionInstruction),
	); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf("expected connection instructions removal, got %v", statErr)
	}
}

//nolint:paralleltest // modifies package-level hooks for the local runtime.
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
	var waitGroup sync.WaitGroup
	errCh := make(chan error, 2)
	for _, deploymentDir := range []string{deploymentA, deploymentB} {
		waitGroup.Add(1)
		go func(dir string) {
			defer waitGroup.Done()
			errCh <- Deploy(
				context.Background(),
				config.NewDeploymentDir(dir),
				false,
				TofuLockfileReadonly,
			)
		}(deploymentDir)
	}
	waitGroup.Wait()
	close(errCh)

	// Then
	for err := range errCh {
		if err != nil {
			t.Fatalf("expected concurrent deploy to succeed, got %v", err)
		}
	}
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentA)
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentB)

	infoA := mustReadLocalDeploymentInfo(t, deploymentA)
	infoB := mustReadLocalDeploymentInfo(t, deploymentB)
	if infoA.Runtime.RuntimeRoot == infoB.Runtime.RuntimeRoot {
		t.Fatalf("expected distinct runtime roots, got %q", infoA.Runtime.RuntimeRoot)
	}
	if infoA.Runtime.PIDFilePath == infoB.Runtime.PIDFilePath {
		t.Fatalf("expected distinct pid paths, got %q", infoA.Runtime.PIDFilePath)
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

	if _, infoErr := config.ReadDeploymentInfo(
		config.NewDeploymentDir(deploymentB),
	); infoErr != nil {
		t.Fatalf("expected deployment B info to remain available, got %v", infoErr)
	}
	assertWorkflowStateType[*config.WorkflowStateRunning](t, deploymentB)

	// Cleanup
	if err := Destroy(
		context.Background(),
		config.NewDeploymentDir(deploymentB),
		false,
	); err != nil {
		t.Fatalf("expected cleanup destroy for deployment B to succeed, got %v", err)
	}
}

func installLocalLifecycleTestHooks(t *testing.T) func() {
	t.Helper()

	originalPlatformSupport := localRuntimePlatformSupported
	originalRunnerCommand := localRunnerCommand
	originalVerifyDatabaseConnection := verifyDatabaseConnectionFn
	originalEnsureLocalDatabaseCredentials := ensureLocalDatabaseCredentialsFn

	var (
		processesMu sync.Mutex
		processes   []int
	)

	localRuntimePlatformSupported = func() bool { return true }
	verifyDatabaseConnectionFn = func(context.Context, config.DeploymentDir) error {
		return nil
	}
	ensureLocalDatabaseCredentialsFn = func(context.Context, config.DeploymentDir) error {
		return nil
	}
	localRunnerCommand = func(deployment config.DeploymentDir, _ *localruntime.Runtime) error {
		pid, err := startLocalLifecycleHelperProcess(deployment.Root())
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
		ensureLocalDatabaseCredentialsFn = originalEnsureLocalDatabaseCredentials

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
	diskImagePath := filepath.Join(fixtureDir, "exasol-nano-vm.img")
	if err := os.WriteFile(diskImagePath, []byte("disk-image"), 0o600); err != nil {
		t.Fatalf("expected fake disk image fixture, got %v", err)
	}
	runPath := filepath.Join(fixtureDir, "exasol-nano-db.run")
	if err := os.WriteFile(runPath, []byte("run-binary"), 0o600); err != nil {
		t.Fatalf("expected fake run binary fixture, got %v", err)
	}

	if err := runtime.SaveState(&localstate.State{
		Ports: map[string]int{
			"db": 18563,
			"ui": 18443,
		},
		Payload: &localstate.PayloadRef{
			Version:       "1.2.3",
			Architecture:  "arm64",
			Checksum:      "abc",
			DiskImagePath: diskImagePath,
			RunPath:       runPath,
		},
	}); err != nil {
		t.Fatalf("expected local runtime state to be saved, got %v", err)
	}
}

func startLocalLifecycleHelperProcess(deploymentDir string) (int, error) {
	runtime := localruntime.New(deploymentDir)
	cmd := exec.CommandContext(
		context.Background(),
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

func mustReadLocalDeploymentInfo(t *testing.T, deploymentDir string) *config.DeploymentInfo {
	t.Helper()

	info, err := config.ReadDeploymentInfo(config.NewDeploymentDir(deploymentDir))
	if err != nil {
		t.Fatalf("expected deployment info, got %v", err)
	}
	if info.Backend != config.DeploymentBackendLocal || info.Runtime == nil {
		t.Fatalf("expected local deployment info, got %#v", info)
	}

	return info
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}

	return true
}
