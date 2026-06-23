// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/exasol/exasol-personal/assets/localruntimebin"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/util"
)

const (
	DirName            = "local"
	runtimeDirName     = "runtime"
	RunnerFileName     = localruntimebin.RunnerBinaryName
	vmDirName          = "vm"
	vmStateFileName    = "vm-state.json"
	vmPIDFileName      = "vm.pid"
	PrivateKeyFileName = "node_access.pem"
	stopPollInterval   = 500 * time.Millisecond
	stopTimeout        = 90 * time.Second
	dirMode            = 0o750
	privateFileMode    = 0o600
	executableFileMode = 0o700
	maxTCPPort         = 65535
)

type Config struct {
	CPUCount   int
	MemoryMB   int
	DataSizeGB int
	Ports      string
}

type State struct {
	VMIP                   string
	SSHPort                int
	DBPort                 int
	UIPort                 int
	PrivateKeyPath         string
	PrivateKeyRelativePath string
}

type Paths struct {
	Root           string
	WorkDir        string
	RunnerPath     string
	VMDir          string
	StatePath      string
	PrivateKeyPath string
}

func NewPaths(deployment config.DeploymentDir) Paths {
	root := deployment.Resolve(DirName)
	workDir := filepath.Join(root, runtimeDirName)

	return Paths{
		Root:           root,
		WorkDir:        workDir,
		RunnerPath:     filepath.Join(workDir, RunnerFileName),
		VMDir:          filepath.Join(workDir, vmDirName),
		StatePath:      filepath.Join(workDir, vmStateFileName),
		PrivateKeyPath: filepath.Join(root, PrivateKeyFileName),
	}
}

func (paths Paths) PrivateKeyRelativePath(deployment config.DeploymentDir) (string, error) {
	rel, err := filepath.Rel(deployment.Root(), paths.PrivateKeyPath)
	if err != nil {
		return "", err
	}

	return filepath.ToSlash(rel), nil
}

type runnerPorts struct {
	SSH int `json:"ssh"`
	DB  int `json:"db"`
	UI  int `json:"ui"`
}

//nolint:tagliatelle // Runner state JSON keys are defined by the runner contract.
type runnerState struct {
	VMName    string      `json:"vm_name"`
	VMIP      string      `json:"vm_ip"`
	CPUCount  string      `json:"cpu_count"`
	RAMSize   string      `json:"ram_size"`
	PID       string      `json:"pid"`
	SharedDir string      `json:"shared_dir"`
	Ports     runnerPorts `json:"ports"`
}

func Prepare(
	ctx context.Context,
	deployment config.DeploymentDir,
	runtimeConfig Config,
	out, outErr io.Writer,
) error {
	runtime := newRuntime(deployment, runtimeConfig)
	return runtime.prepare(ctx, out, outErr)
}

func RunCommand(
	ctx context.Context,
	deployment config.DeploymentDir,
	args []string,
	out, outErr io.Writer,
) error {
	runtime := newRuntime(deployment, Config{})
	return runtime.runnerCommand(ctx, args, out, outErr)
}

func ReadState(deployment config.DeploymentDir) (*State, error) {
	runtime := newRuntime(deployment, Config{})
	state, err := readRunnerState(runtime.paths.StatePath)
	if err != nil {
		return nil, err
	}

	return runtime.toState(state)
}

func Stop(ctx context.Context, deployment config.DeploymentDir, out, outErr io.Writer) error {
	runtime := newRuntime(deployment, Config{})
	if err := runtime.ensureRunnerExecutable(); err != nil {
		return err
	}

	if err := runtime.runnerCommand(ctx, []string{"stop"}, out, outErr); err != nil {
		return err
	}

	return runtime.waitForDaemonExit(ctx)
}

func Destroy(ctx context.Context, deployment config.DeploymentDir, out, outErr io.Writer) error {
	runtime := newRuntime(deployment, Config{})
	if _, err := os.Stat(runtime.paths.RunnerPath); err == nil {
		if err := runtime.runnerCommand(ctx, []string{"stop"}, out, outErr); err == nil {
			if err := runtime.waitForDaemonExit(ctx); err != nil {
				return err
			}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local runner: %w", err)
	}

	if err := os.RemoveAll(runtime.paths.Root); err != nil {
		return fmt.Errorf("failed to remove local runtime files %s: %w", runtime.paths.Root, err)
	}

	return nil
}

type localRuntime struct {
	deployment    config.DeploymentDir
	paths         Paths
	runtimeConfig Config
}

func newRuntime(deployment config.DeploymentDir, runtimeConfig Config) *localRuntime {
	return &localRuntime{
		deployment:    deployment,
		paths:         NewPaths(deployment),
		runtimeConfig: runtimeConfig,
	}
}

func (runtime *localRuntime) prepare(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	if err := os.MkdirAll(runtime.paths.WorkDir, dirMode); err != nil {
		return fmt.Errorf("failed to create local runtime directory: %w", err)
	}
	if err := runtime.ensureRunnerExecutable(); err != nil {
		return err
	}
	if err := runtime.ensureSSHKey(); err != nil {
		return err
	}

	return runtime.initializeVMIfNeeded(ctx, out, outErr)
}

func (runtime *localRuntime) initializeVMIfNeeded(
	ctx context.Context,
	out, outErr io.Writer,
) error {
	if _, err := os.Stat(runtime.paths.VMDir); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local VM directory: %w", err)
	}

	return runtime.runnerCommand(
		ctx,
		[]string{"init", "--ssh-key", runtime.paths.PrivateKeyPath},
		out,
		outErr,
	)
}

func (runtime *localRuntime) toState(state *runnerState) (*State, error) {
	keyFile, err := runtime.paths.PrivateKeyRelativePath(runtime.deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local SSH key path: %w", err)
	}

	return &State{
		VMIP:                   state.VMIP,
		SSHPort:                state.Ports.SSH,
		DBPort:                 state.Ports.DB,
		UIPort:                 state.Ports.UI,
		PrivateKeyPath:         runtime.paths.PrivateKeyPath,
		PrivateKeyRelativePath: keyFile,
	}, nil
}

func (runtime *localRuntime) ensureRunnerExecutable() error {
	return writeEmbeddedRunner(runtime.paths.RunnerPath)
}

func writeEmbeddedRunner(targetPath string) error {
	if info, err := os.Stat(targetPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("local runner target is a directory: %s", targetPath)
		}

		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local runner %s: %w", targetPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), dirMode); err != nil {
		return fmt.Errorf("failed to create local runner target directory: %w", err)
	}
	if err := localruntimebin.WriteBinary(targetPath); err != nil {
		return fmt.Errorf("failed to write embedded local runner %s: %w", targetPath, err)
	}

	return nil
}

func (runtime *localRuntime) runnerCommand(
	ctx context.Context,
	args []string,
	out, outErr io.Writer,
) error {
	if len(args) == 0 {
		return errors.New("local runner command is empty")
	}

	cmd := exec.CommandContext(ctx, runtime.paths.RunnerPath, args...)
	cmd.Dir = runtime.paths.WorkDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = util.CombineWriters(&stdout, out)
	cmd.Stderr = util.CombineWriters(&stderr, outErr)

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if detail != "" {
			return fmt.Errorf("local runner command %q failed: %w\n%s", args[0], err, detail)
		}

		return fmt.Errorf("local runner command %q failed: %w", args[0], err)
	}

	return nil
}

func (runtime *localRuntime) waitForDaemonExit(ctx context.Context) error {
	pid, err := runtime.readDaemonPID()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}
	if !processRunning(pid) {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()

	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf(
				"local runner daemon did not exit within %s after stop signal",
				stopTimeout,
			)
		case <-ticker.C:
			if !processRunning(pid) {
				return nil
			}
		}
	}
}

func (runtime *localRuntime) readDaemonPID() (int, error) {
	pidPath := filepath.Join(runtime.paths.WorkDir, vmPIDFileName)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return 0, fmt.Errorf("failed to parse local runner daemon PID from %s: %w", pidPath, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("local runner daemon PID must be greater than zero: %d", pid)
	}

	return pid, nil
}

func processRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}

	return false
}

func (runtime *localRuntime) ensureSSHKey() error {
	if _, err := os.Stat(runtime.paths.PrivateKeyPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect local SSH key: %w", err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate local SSH key: %w", err)
	}
	privateKeyPKCS8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal local SSH private key: %w", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyPKCS8,
	})
	if err := os.MkdirAll(filepath.Dir(runtime.paths.PrivateKeyPath), dirMode); err != nil {
		return fmt.Errorf("failed to create local SSH key directory: %w", err)
	}
	if err := os.WriteFile(
		runtime.paths.PrivateKeyPath,
		privateKeyPEM,
		privateFileMode,
	); err != nil {
		return fmt.Errorf("failed to write local SSH private key: %w", err)
	}

	return nil
}

func readRunnerState(statePath string) (*runnerState, error) {
	stateFile, err := os.Open(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local VM state file %s: %w", statePath, err)
	}
	defer stateFile.Close()

	var state runnerState
	if err := json.NewDecoder(stateFile).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to parse local VM state file %s: %w", statePath, err)
	}
	if err := validateRunnerState(&state); err != nil {
		return nil, err
	}

	return &state, nil
}

func validateRunnerState(state *runnerState) error {
	if state == nil {
		return errors.New("local VM state is missing")
	}
	if err := validatePort("ssh", state.Ports.SSH); err != nil {
		return err
	}
	if err := validatePort("database", state.Ports.DB); err != nil {
		return err
	}
	if state.Ports.UI != 0 {
		return validatePort("ui", state.Ports.UI)
	}

	return nil
}

func validatePort(name string, port int) error {
	if port <= 0 || port > maxTCPPort {
		return fmt.Errorf("local VM state contains invalid %s port: %d", name, port)
	}

	return nil
}
