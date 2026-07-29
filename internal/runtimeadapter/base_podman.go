// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type BasePodmanAdapter struct {
	DeploymentRoot string
	Commands       CommandRunner
}

func NewBasePodmanAdapter(deploymentRoot string, commands CommandRunner) *BasePodmanAdapter {
	if commands == nil {
		commands = OSCommandRunner{}
	}

	return &BasePodmanAdapter{DeploymentRoot: deploymentRoot, Commands: commands}
}

func (adapter *BasePodmanAdapter) Prerequisites(ctx context.Context,
	options PrerequisiteOptions,
) error {
	return nil
}

func (adapter *BasePodmanAdapter) Start(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) (*RuntimeStatus, error) {
	// FIXME Add internal / external DataPath maybe?
	//
	// if err := os.MkdirAll(spec.DataPath, privateDirMode); err != nil {
	// 	return nil, fmt.Errorf("failed to create Personal-owned data path: %w", err)
	// }
	manifestPath, imagePath, err := adapter.stage(spec, true)
	if err != nil {
		return nil, err
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		"podman",
		"load",
		"--input",
		imagePath,
	); err != nil {
		return nil, fmt.Errorf("failed to load embedded Nano image: %w", err)
	}
	if err := adapter.ensureSLCImages(ctx, spec.SLCMounts, stdout, stderr); err != nil {
		return nil, err
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		"podman",
		"kube",
		"play",
		"--replace",
		manifestPath,
	); err != nil {
		return nil, fmt.Errorf("failed to apply Nano workload: %w", err)
	}

	return adapter.Status(ctx, spec)
}

func (adapter *BasePodmanAdapter) Stop(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	status, err := adapter.Status(ctx, spec)
	if err != nil {
		return err
	}
	if status.Phase == RuntimePhaseStopped {
		return nil
	}
	manifestPath, _, err := adapter.stage(spec, false)
	if err != nil {
		return err
	}
	if err := adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		"podman",
		"kube",
		"down",
		manifestPath,
	); err != nil {
		return fmt.Errorf("failed to stop Nano workload: %w", err)
	}

	return nil
}

func (adapter *BasePodmanAdapter) Status(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	name := WorkloadName(spec.DeploymentID)
	if err := adapter.Commands.Run(
		ctx,
		nil,
		io.Discard,
		io.Discard,
		"podman",
		"pod",
		"exists",
		name,
	); err != nil {
		if exitCode := commandExitCode(err); exitCode != 0 {
			return nil, fmt.Errorf("failed to check Podman workload with code: %d", exitCode)
		} else {
			return nil, fmt.Errorf("failed to check Podman workload: %w", err)
		}

		return &RuntimeStatus{
			Phase:        RuntimePhaseStopped,
			WorkloadName: name,
			Database: RuntimeEndpoint{
				Address: "127.0.0.1",
				Port:    spec.DBHostPort,
			},
		}, nil
	}
	data, err := adapter.Commands.Output(
		ctx,
		"podman",
		"pod",
		"inspect",
		"--format",
		"{{.State}}",
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect Podman workload: %w", err)
	}
	workloadState := strings.TrimSpace(string(data))
	phase := RuntimePhaseDegraded
	if strings.EqualFold(workloadState, "Running") {
		phase = RuntimePhaseRunning
	}
	containerName := name + "-db"
	containerIDData, _ := adapter.Commands.Output(
		ctx,
		"podman",
		"container",
		"inspect",
		"--format",
		"{{.Id}}",
		containerName,
	)
	resolvedPort := spec.DBHostPort
	if portData, portErr := adapter.Commands.Output(
		ctx,
		"podman",
		"port",
		containerName,
		strconv.Itoa(NanoContainerPort)+"/tcp",
	); portErr == nil {
		if port := parsePublishedPort(string(portData)); port > 0 {
			resolvedPort = port
		}
	}

	return &RuntimeStatus{
		Phase:        phase,
		WorkloadName: name,
		Database: RuntimeEndpoint{
			Address: "127.0.0.1",
			Port:    resolvedPort,
		},
		ContainerID: strings.TrimSpace(string(containerIDData)),
	}, nil
}

func (adapter *BasePodmanAdapter) Health(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.Status(ctx, spec)
}

func (adapter *BasePodmanAdapter) Logs(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.Commands.Run(
		ctx,
		nil,
		stdout,
		stderr,
		"podman",
		"logs",
		WorkloadName(spec.DeploymentID)+"-db",
	)
}

func (adapter *BasePodmanAdapter) Destroy(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.Stop(ctx, spec, stdout, stderr)
}

//nolint:revive // The two string results are the manifest and image paths.
func (adapter *BasePodmanAdapter) stage(
	spec WorkloadSpec,
	includeImage bool,
) (string, string, error) {
	controlPath := filepath.Join(adapter.DeploymentRoot, "local", "control")
	manifestPath := filepath.Join(controlPath, "workload.yaml")
	imagePath := filepath.Join(controlPath, "nano-image.tar.gz")
	imageStage := stageManifestOnly
	if includeImage {
		imageStage = stageManifestAndImage
	}
	if err := stageWorkloadAssets(spec, manifestPath, imagePath, imageStage); err != nil {
		return "", "", err
	}

	return manifestPath, imagePath, nil
}

func (adapter *BasePodmanAdapter) ensureSLCImages(
	ctx context.Context,
	mounts []SLCMount,
	stdout, stderr io.Writer,
) error {
	seen := make(map[string]struct{}, len(mounts))
	for _, mount := range mounts {
		seen[mount.Image] = struct{}{}
	}
	if len(seen) > 0 {
		args := []string{
			"pull",
			"--policy=missing",
		}
		for slc := range maps.Keys(seen) {
			args = append(args, slc)
		}
		if err := adapter.Commands.Run(
			ctx,
			nil,
			stdout,
			stderr,
			"podman",
			args...,
		); err != nil {
			return fmt.Errorf("failed to pull SLC images %w", err)
		}
	}

	return nil
}

func commandExitCode(err error) int {
	var withExitCode *exec.ExitError
	if errors.As(err, &withExitCode) {
		return withExitCode.ExitCode()
	}

	return -1
}
