// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	localVMConfigSchema = 1
	localVMHookAPI      = 1
	localVMSSHPort      = 22
	providerPhaseLive   = "running"
)

type MacVMAdapter struct {
	DeploymentRoot  string
	LocalVMBinary   string
	ExpectedVersion string
	Commands        CommandRunner
}

type macRuntimePaths struct {
	Root     string
	Control  string
	Data     string
	State    string
	Config   string
	Manifest string
	Helper   string
	Hook     string
	Image    string
}

func NewMacVMAdapter(deploymentRoot, localVMBinary string, commands CommandRunner) *MacVMAdapter {
	if commands == nil {
		commands = OSCommandRunner{}
	}

	return &MacVMAdapter{
		DeploymentRoot: deploymentRoot,
		LocalVMBinary:  localVMBinary,
		Commands:       commands,
	}
}

func (*MacVMAdapter) Capabilities() RuntimeCapabilities {
	return PlatformCapabilities("darwin")
}

func (adapter *MacVMAdapter) Prerequisites(
	ctx context.Context,
	_ PrerequisiteOptions,
) error {
	if strings.TrimSpace(adapter.LocalVMBinary) == "" {
		return errors.New("local-vm binary path is missing")
	}
	data, err := adapter.Commands.Output(
		ctx,
		adapter.LocalVMBinary,
		"version",
		"--json",
	)
	if err != nil {
		return fmt.Errorf("local-vm prerequisite check failed: %w", err)
	}
	var version localVMVersion
	if err := json.Unmarshal(data, &version); err != nil {
		return fmt.Errorf("failed to parse local-vm version contract: %w", err)
	}
	if adapter.ExpectedVersion != "" && version.Version != adapter.ExpectedVersion {
		return fmt.Errorf(
			"local-vm version %q does not match the release-pinned version %q",
			version.Version,
			adapter.ExpectedVersion,
		)
	}
	if version.ConfigSchemaVersion != localVMConfigSchema ||
		version.HookAPIVersion != localVMHookAPI ||
		version.StateSchemaVersion != 1 {
		return fmt.Errorf(
			"unsupported local-vm contract: config=%d hook=%d state=%d; expected 1/1/1",
			version.ConfigSchemaVersion,
			version.HookAPIVersion,
			version.StateSchemaVersion,
		)
	}

	return nil
}

type localVMVersion struct {
	Version             string `json:"version"`
	ConfigSchemaVersion int    `json:"configSchemaVersion"`
	//nolint:tagliatelle // hookAPIVersion is part of the local-vm v2 contract.
	HookAPIVersion     int `json:"hookAPIVersion"`
	StateSchemaVersion int `json:"stateSchemaVersion"`
}

func (adapter *MacVMAdapter) Start(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) (*RuntimeStatus, error) {
	paths := adapter.paths()
	if err := adapter.stage(spec, paths, true); err != nil {
		return nil, err
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		adapter.LocalVMBinary,
		"init",
		"--state-dir",
		paths.State,
		"--config",
		paths.Config,
	); err != nil {
		return nil, fmt.Errorf("failed to initialize local VM: %w", err)
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		adapter.LocalVMBinary,
		"start",
		"--state-dir",
		paths.State,
		"--config",
		paths.Config,
	); err != nil {
		return nil, fmt.Errorf("failed to start local VM workload: %w", err)
	}

	return adapter.Status(ctx, spec)
}

func (adapter *MacVMAdapter) Stop(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	state, stateErr := adapter.providerStatus(ctx)
	if stateErr == nil && state.Phase == providerPhaseLive {
		if err := adapter.stage(spec, adapter.paths(), false); err != nil {
			return err
		}
		if err := adapter.runRemoteHelper(ctx, state, "down", stdout, stderr); err != nil {
			return fmt.Errorf("failed to stop workload before VM shutdown: %w", err)
		}
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		adapter.LocalVMBinary,
		"stop",
		"--state-dir",
		adapter.paths().State,
	); err != nil {
		return fmt.Errorf("failed to stop local VM: %w", err)
	}

	return nil
}

func (adapter *MacVMAdapter) Status(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.runtimeStatus(ctx, spec, false)
}

func (adapter *MacVMAdapter) Health(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.runtimeStatus(ctx, spec, true)
}

//nolint:funcorder,revive // Shared implementation for the two exported observation methods.
func (adapter *MacVMAdapter) runtimeStatus(
	ctx context.Context,
	spec WorkloadSpec,
	probeHealth bool,
) (*RuntimeStatus, error) {
	var state *localVMProviderState
	var err error
	if probeHealth {
		state, err = adapter.providerHealth(ctx)
	} else {
		state, err = adapter.providerStatus(ctx)
	}
	if err != nil {
		return nil, err
	}
	status := &RuntimeStatus{
		Phase:        RuntimePhaseStopped,
		WorkloadName: WorkloadName(spec.DeploymentID),
		VM: &VMDetails{
			Phase:          state.Phase,
			PID:            state.PID,
			GuestIP:        state.GuestIP,
			PrivateKeyPath: state.PrivateKeyPath,
			Forwards:       map[string]RuntimeEndpoint{},
			Hook:           state.Hook.Phase,
		},
	}
	if state.SSH != nil {
		status.VM.SSH = &RuntimeEndpoint{
			Address: state.SSH.Address,
			Port:    state.SSH.Port,
		}
	}
	for _, forward := range state.Forwards {
		endpoint := RuntimeEndpoint{
			Address: forward.HostAddress,
			Port:    forward.HostPort,
			Health:  forward.Health,
		}
		status.VM.Forwards[forward.Name] = endpoint
		if forward.Name == "database" {
			status.Database = endpoint
		}
	}
	if state.Phase != providerPhaseLive {
		switch state.Phase {
		case "starting":
			status.Phase = RuntimePhaseStarting
		case "stopped", "initialized":
			status.Phase = RuntimePhaseStopped
		default:
			status.Phase = RuntimePhaseDegraded
			status.Message = state.Message
			if status.Message == "" {
				status.Message = fmt.Sprintf(
					"local VM provider reports phase %q",
					state.Phase,
				)
			}
		}

		return status, nil
	}
	if state.Hook.Phase != "succeeded" {
		status.Phase = RuntimePhaseDegraded
		status.Message = "local VM is running but the workload hook did not succeed"

		return status, nil
	}
	workloadState, err := adapter.remoteWorkloadState(ctx, state)
	if err != nil {
		status.Phase = RuntimePhaseDegraded
		status.Message = fmt.Sprintf("failed to inspect workload over SSH: %v", err)

		return status, nil
	}
	if !strings.EqualFold(workloadState, "running") {
		status.Phase = RuntimePhaseDegraded
		status.Message = fmt.Sprintf("Podman reports workload state %q", workloadState)

		return status, nil
	}
	status.Phase = RuntimePhaseRunning

	return status, nil
}

func (adapter *MacVMAdapter) Logs(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	state, err := adapter.providerStatus(ctx)
	if err != nil {
		return err
	}
	if err := adapter.stage(spec, adapter.paths(), false); err != nil {
		return err
	}

	return adapter.runRemoteHelper(ctx, state, "logs", stdout, stderr)
}

func (adapter *MacVMAdapter) Destroy(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	state, err := adapter.providerStatus(ctx)
	if err == nil && state.Phase == providerPhaseLive {
		if stageErr := adapter.stage(spec, adapter.paths(), false); stageErr != nil {
			return stageErr
		}
		if downErr := adapter.runRemoteHelper(ctx, state, "down", stdout, stderr); downErr != nil {
			return fmt.Errorf("failed to remove workload before VM destroy: %w", downErr)
		}
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		adapter.LocalVMBinary,
		"destroy",
		"--state-dir",
		adapter.paths().State,
	); err != nil {
		return fmt.Errorf("failed to destroy local VM: %w", err)
	}

	return nil
}

func (adapter *MacVMAdapter) Shell(
	ctx context.Context,
	spec WorkloadSpec,
	kind ShellKind,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	state, err := adapter.providerStatus(ctx)
	if err != nil {
		return err
	}
	args, err := adapter.sshArgs(state)
	if err != nil {
		return err
	}
	args = append([]string{"-tt"}, args...)
	switch kind {
	case ShellVM:
		// No remote command requests an interactive root VM shell.
	case ShellContainer:
		name := WorkloadName(spec.DeploymentID) + "-db"
		command := "rootfs=$(podman mount " + shellQuote(name) + ") && " +
			"pid=$(podman inspect " + shellQuote(name) + " --format '{{.State.Pid}}') && " +
			"cd \"$rootfs\" && exec nsenter --target \"$pid\" --uts --ipc --net /bin/sh"
		args = append(args, command)
	default:
		return fmt.Errorf("unsupported shell kind %q", kind)
	}

	return adapter.Commands.Run(ctx, stdin, stdout, stderr, "ssh", args...)
}

func (adapter *MacVMAdapter) paths() macRuntimePaths {
	root := filepath.Join(adapter.DeploymentRoot, "local")
	control := filepath.Join(root, "control")
	data := filepath.Join(root, "data")

	return macRuntimePaths{
		Root:     root,
		Control:  control,
		Data:     data,
		State:    filepath.Join(root, "runtime"),
		Config:   filepath.Join(root, "generated", "local-vm.json"),
		Manifest: filepath.Join(control, "workload.yaml"),
		Helper:   filepath.Join(control, "workload-helper"),
		Hook:     filepath.Join(control, "hooks", "start"),
		Image:    filepath.Join(control, "nano-image.tar.gz"),
	}
}

type localVMConfig struct {
	SchemaVersion int              `json:"schemaVersion"`
	Resources     localVMResources `json:"resources"`
	Shares        []localVMShare   `json:"shares"`
	Forwards      []localVMForward `json:"forwards"`
	BootHook      localVMBootHook  `json:"bootHook"`
}

type localVMResources struct {
	CPUs      int `json:"cpus"`
	MemoryMiB int `json:"memoryMiB"`
}

type localVMShare struct {
	Name      string `json:"name"`
	HostPath  string `json:"hostPath"`
	GuestPath string `json:"guestPath"`
	ReadOnly  bool   `json:"readOnly"`
}

type localVMForward struct {
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	HostAddress string `json:"hostAddress"`
	HostPort    int    `json:"hostPort"`
	GuestPort   int    `json:"guestPort"`
}

type localVMBootHook struct {
	APIVersion int    `json:"apiVersion"`
	Share      string `json:"share"`
	Path       string `json:"path"`
}

//nolint:revive // The flag prevents read-only operations from loading the large image archive.
func (*MacVMAdapter) stage(spec WorkloadSpec, paths macRuntimePaths, includeImage bool) error {
	runtimePaths := []string{
		paths.Control,
		paths.Data,
		paths.State,
		filepath.Dir(paths.Config),
	}
	for _, path := range runtimePaths {
		if err := os.MkdirAll(path, privateDirMode); err != nil {
			return fmt.Errorf("failed to create local runtime path %s: %w", path, err)
		}
	}
	guestSpec := spec
	guestSpec.DBHostPort = NanoContainerPort
	guestSpec.DBHostAddress = "0.0.0.0"
	guestSpec.DataPath = "/mnt/data/exa"
	helper, err := RenderWorkloadHelper(
		guestSpec,
		"/mnt/control/workload.yaml",
		"/mnt/control/nano-image.tar.gz",
	)
	if err != nil {
		return err
	}
	hook := RenderMacBootHook(
		"/mnt/data",
		"/var/lib/exa",
		"/mnt/control/workload-helper",
	)
	imageStage := stageManifestOnly
	if includeImage {
		imageStage = stageManifestAndImage
	}
	if err := stageWorkloadAssets(
		guestSpec,
		paths.Manifest,
		paths.Image,
		imageStage,
	); err != nil {
		return err
	}
	if err := writeAtomic(paths.Helper, helper, executableMode); err != nil {
		return err
	}
	if err := writeAtomic(paths.Hook, hook, executableMode); err != nil {
		return err
	}
	vmConfig := localVMConfig{
		SchemaVersion: localVMConfigSchema,
		Resources: localVMResources{
			CPUs:      spec.CPUs,
			MemoryMiB: spec.MemoryMiB,
		},
		Shares: []localVMShare{{
			Name:      "control",
			HostPath:  paths.Control,
			GuestPath: "/mnt/control",
		}},
		Forwards: []localVMForward{
			{
				Name:        "ssh",
				Protocol:    "tcp",
				HostAddress: "127.0.0.1",
				HostPort:    0,
				GuestPort:   localVMSSHPort,
			},
			{
				Name:        "database",
				Protocol:    "tcp",
				HostAddress: "127.0.0.1",
				HostPort:    spec.DBHostPort,
				GuestPort:   NanoContainerPort,
			},
		},
		BootHook: localVMBootHook{
			APIVersion: localVMHookAPI,
			Share:      "control",
			Path:       "hooks/start",
		},
	}
	vmConfig.Shares = append(vmConfig.Shares, localVMShare{
		Name:      "data",
		HostPath:  paths.Data,
		GuestPath: "/mnt/data",
	})
	configData, err := json.MarshalIndent(vmConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode local-vm config: %w", err)
	}

	return writeAtomic(paths.Config, configData, privateFileMode)
}

type localVMProviderState struct {
	SchemaVersion int    `json:"schemaVersion"`
	Phase         string `json:"phase"`
	Message       string `json:"message"`
	PID           int    `json:"pid"`
	//nolint:tagliatelle // guestIP is part of the local-vm v2 state contract.
	GuestIP        string                `json:"guestIP"`
	SSH            *localVMEndpoint      `json:"ssh"`
	PrivateKeyPath string                `json:"privateKeyPath"`
	Forwards       []localVMForwardState `json:"forwards"`
	Hook           localVMHookState      `json:"hook"`
}

type localVMEndpoint struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

type localVMForwardState struct {
	Name        string `json:"name"`
	HostAddress string `json:"hostAddress"`
	HostPort    int    `json:"hostPort"`
	Health      string `json:"health"`
}

type localVMHookState struct {
	Phase string `json:"phase"`
}

func (adapter *MacVMAdapter) providerStatus(ctx context.Context) (*localVMProviderState, error) {
	return adapter.providerState(ctx, "status")
}

func (adapter *MacVMAdapter) providerHealth(ctx context.Context) (*localVMProviderState, error) {
	return adapter.providerState(ctx, "health-check")
}

func (adapter *MacVMAdapter) providerState(
	ctx context.Context,
	command string,
) (*localVMProviderState, error) {
	data, err := adapter.Commands.Output(
		ctx,
		adapter.LocalVMBinary,
		command,
		"--state-dir",
		adapter.paths().State,
		"--json",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query local-vm %s: %w", command, err)
	}
	var state localVMProviderState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse local-vm status: %w", err)
	}
	if state.SchemaVersion != 1 {
		return nil, fmt.Errorf(
			"unsupported local-vm state schema %d; expected 1",
			state.SchemaVersion,
		)
	}

	return &state, nil
}

func (adapter *MacVMAdapter) runRemoteHelper(
	ctx context.Context,
	state *localVMProviderState,
	mode string,
	stdout, stderr io.Writer,
) error {
	args, err := adapter.sshArgs(state)
	if err != nil {
		return err
	}
	args = append(args, "/mnt/control/workload-helper", mode)

	return adapter.Commands.Run(ctx, nil, stdout, stderr, "ssh", args...)
}

func (adapter *MacVMAdapter) remoteWorkloadState(
	ctx context.Context,
	state *localVMProviderState,
) (string, error) {
	args, err := adapter.sshArgs(state)
	if err != nil {
		return "", err
	}
	args = append(args, "/mnt/control/workload-helper", "status")
	data, err := adapter.Commands.Output(ctx, "ssh", args...)
	if err != nil {
		return "", err
	}
	workloadState := strings.TrimSpace(string(data))
	if workloadState == "" {
		return "", errors.New("podman workload status has no state")
	}

	return workloadState, nil
}

func (*MacVMAdapter) sshArgs(state *localVMProviderState) ([]string, error) {
	if state == nil || state.SSH == nil || state.SSH.Port <= 0 {
		return nil, errors.New("local-vm state has no SSH endpoint")
	}
	if strings.TrimSpace(state.PrivateKeyPath) == "" {
		return nil, errors.New("local-vm state has no private key path")
	}

	return []string{
		"-i", state.PrivateKeyPath,
		"-p", strconv.Itoa(state.SSH.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"root@" + state.SSH.Address,
	}, nil
}
