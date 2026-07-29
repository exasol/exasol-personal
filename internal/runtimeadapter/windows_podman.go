// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package runtimeadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"
)

var windowsDrivePath = regexp.MustCompile(`^([A-Za-z]):[\\/](.*)$`)

var minimumPodmanVersion = semver.MustParse("5.8.0")

type WindowsPodmanAdapter struct {
	DeploymentRoot string
	Commands       CommandRunner
}

func NewWindowsPodmanAdapter(deploymentRoot string, commands CommandRunner) *WindowsPodmanAdapter {
	if commands == nil {
		commands = OSCommandRunner{}
	}

	return &WindowsPodmanAdapter{DeploymentRoot: deploymentRoot, Commands: commands}
}

func (*WindowsPodmanAdapter) Capabilities() RuntimeCapabilities {
	return PlatformCapabilities("windows")
}

func (adapter *WindowsPodmanAdapter) Prerequisites(
	ctx context.Context,
	options PrerequisiteOptions,
) error {
	wslStatus, err := adapter.Commands.Output(ctx, "wsl.exe", "--status")
	if err != nil {
		return fmt.Errorf("WSL2 is required for local deployments: %w", err)
	}
	if !reportsWSL2(wslStatus) {
		return errors.New(
			"WSL2 is required for local deployments; " +
				"`wsl.exe --status` does not report default version 2",
		)
	}
	if _, err := adapter.Commands.LookPath("podman"); err != nil {
		if err := requireInteractiveApproval(
			options,
			"Podman is required. Install it for the current user with Winget?",
		); err != nil {
			return err
		}
		if err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"winget",
			"install",
			"--id",
			"RedHat.Podman",
			"--exact",
			"--scope",
			"user",
			"--accept-package-agreements",
			"--accept-source-agreements",
		); err != nil {
			return fmt.Errorf("failed to install Podman with Winget: %w", err)
		}
	}
	if err := ensureWindowsMachine(ctx, adapter); err != nil {
		return err
	}
	versions, err := readPodmanVersions(ctx, adapter.Commands)
	if err != nil {
		return err
	}
	if versions.Client.LT(minimumPodmanVersion) ||
		versions.Server.LT(minimumPodmanVersion) {
		if err := requireInteractiveApproval(
			options,
			fmt.Sprintf(
				"Podman client/server %s/%s is below the validation minimum %s. "+
					"Upgrade it with Winget?",
				versions.Client,
				versions.Server,
				minimumPodmanVersion,
			),
		); err != nil {
			return err
		}
		if err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"winget",
			"upgrade",
			"--id",
			"RedHat.Podman",
			"--exact",
			"--scope",
			"user",
			"--accept-package-agreements",
			"--accept-source-agreements",
		); err != nil {
			return fmt.Errorf("failed to upgrade Podman with Winget: %w", err)
		}
		versions, err = readPodmanVersions(ctx, adapter.Commands)
		if err != nil {
			return fmt.Errorf("failed to verify Podman after Winget upgrade: %w", err)
		}
		if versions.Client.LT(minimumPodmanVersion) ||
			versions.Server.LT(minimumPodmanVersion) {
			return fmt.Errorf(
				"podman client/server remains below the validation minimum after upgrade: %s/%s",
				versions.Client,
				versions.Server,
			)
		}
	}

	return nil
}

func readPodmanVersions(
	ctx context.Context,
	commands CommandRunner,
) (podmanVersions, error) {
	read := func(template, component string) (semver.Version, error) {
		data, err := commands.Output(ctx, "podman", "version", "--format", template)
		if err != nil {
			return semver.Version{}, fmt.Errorf(
				"failed to read Podman %s version: %w",
				component,
				err,
			)
		}
		version, err := semver.ParseTolerant(strings.TrimSpace(string(data)))
		if err != nil {
			return semver.Version{}, fmt.Errorf(
				"failed to parse Podman %s version %q: %w",
				component,
				data,
				err,
			)
		}

		return version, nil
	}
	clientVersion, err := read("{{.Client.Version}}", "client")
	if err != nil {
		return podmanVersions{}, err
	}
	serverVersion, err := read("{{.Server.Version}}", "server")
	if err != nil {
		return podmanVersions{}, err
	}

	return podmanVersions{Client: clientVersion, Server: serverVersion}, nil
}

type podmanVersions struct {
	Client semver.Version
	Server semver.Version
}

func requireInteractiveApproval(options PrerequisiteOptions, prompt string) error {
	if !options.Interactive || options.Confirm == nil {
		return errors.New(prompt + " Re-run interactively to approve this user-scoped change")
	}
	approved, err := options.Confirm(prompt)
	if err != nil {
		return err
	}
	if !approved {
		return errors.New("podman prerequisite change was declined")
	}

	return nil
}

type podmanMachine struct {
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	Name string `json:"Name"`
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	Default bool `json:"Default"`
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	Running bool `json:"Running"`
	//nolint:tagliatelle // Podman emits capitalized JSON property names.
	VMType string `json:"VMType"`
}

func ensureWindowsMachine(ctx context.Context, adapter *WindowsPodmanAdapter) error {
	data, err := adapter.Commands.Output(ctx, "podman", "machine", "list", "--format", "json")
	if err != nil {
		return fmt.Errorf("failed to list Podman machines: %w", err)
	}
	var machines []podmanMachine
	if len(bytes.TrimSpace(data)) != 0 {
		if err := json.Unmarshal(data, &machines); err != nil {
			return fmt.Errorf("failed to parse Podman machine list: %w", err)
		}
	}
	if len(machines) == 0 {
		if err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"podman",
			"machine",
			"init",
		); err != nil {
			return fmt.Errorf("failed to initialize Podman machine: %w", err)
		}

		return adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"podman",
			"machine",
			"start",
		)
	}
	sort.SliceStable(machines, func(left, right int) bool {
		return machines[left].Default && !machines[right].Default
	})
	machine := machines[0]
	if machine.VMType != "" && !strings.EqualFold(machine.VMType, "wsl") {
		return fmt.Errorf("podman machine %q does not use the WSL2 provider", machine.Name)
	}
	if machine.Running {
		return nil
	}

	return adapter.Commands.Run(
		ctx,
		nil,
		io.Discard,
		io.Discard,
		"podman",
		"machine",
		"start",
		machine.Name,
	)
}

func (adapter *WindowsPodmanAdapter) Start(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) (*RuntimeStatus, error) {
	if err := os.MkdirAll(spec.DataPath, privateDirMode); err != nil {
		return nil, fmt.Errorf("failed to create Personal-owned data path: %w", err)
	}
	runtimeSpec, err := windowsRuntimeSpec(spec)
	if err != nil {
		return nil, err
	}
	manifestPath, imagePath, err := adapter.stage(runtimeSpec, true)
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
	if err := adapter.ensureSLCImages(ctx, runtimeSpec.SLCMounts, stdout, stderr); err != nil {
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

func (adapter *WindowsPodmanAdapter) ensureSLCImages(
	ctx context.Context,
	mounts []SLCMount,
	stdout, stderr io.Writer,
) error {
	seen := make(map[string]struct{}, len(mounts))
	for _, mount := range mounts {
		if _, exists := seen[mount.Image]; exists {
			continue
		}
		seen[mount.Image] = struct{}{}
		err := adapter.Commands.Run(
			ctx,
			nil,
			io.Discard,
			io.Discard,
			"podman",
			"image",
			"exists",
			mount.Image,
		)
		if err == nil {
			continue
		}
		if commandExitCode(err) != 1 {
			return fmt.Errorf("failed to inspect SLC image %s: %w", mount.Image, err)
		}
		if err := adapter.Commands.Run(
			ctx,
			nil,
			stdout,
			stderr,
			"podman",
			"pull",
			mount.Image,
		); err != nil {
			return fmt.Errorf("failed to pull SLC image %s: %w", mount.Image, err)
		}
	}

	return nil
}

func (adapter *WindowsPodmanAdapter) Stop(
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
	runtimeSpec, err := windowsRuntimeSpec(spec)
	if err != nil {
		return err
	}
	manifestPath, _, err := adapter.stage(runtimeSpec, false)
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

func (adapter *WindowsPodmanAdapter) Status(
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
		if commandExitCode(err) != 1 {
			return nil, fmt.Errorf("failed to check Podman workload existence: %w", err)
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
	containerName := name + "-nano"
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

func parsePublishedPort(value string) int {
	value = strings.TrimSpace(value)
	index := strings.LastIndex(value, ":")
	if index < 0 {
		return 0
	}
	port, err := strconv.Atoi(value[index+1:])
	if err != nil || port < 1 || port > 65535 {
		return 0
	}

	return port
}

func (adapter *WindowsPodmanAdapter) Health(
	ctx context.Context,
	spec WorkloadSpec,
) (*RuntimeStatus, error) {
	return adapter.Status(ctx, spec)
}

func (adapter *WindowsPodmanAdapter) Logs(
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
		WorkloadName(spec.DeploymentID)+"-nano",
	)
}

func (adapter *WindowsPodmanAdapter) Destroy(
	ctx context.Context,
	spec WorkloadSpec,
	stdout, stderr io.Writer,
) error {
	return adapter.Stop(ctx, spec, stdout, stderr)
}

func (*WindowsPodmanAdapter) Shell(
	context.Context,
	WorkloadSpec,
	ShellKind,
	io.Reader,
	io.Writer,
	io.Writer,
) error {
	return errors.New("VM and container shells are not supported on Windows local deployments")
}

//nolint:revive // The two string results are the manifest and image paths.
func (adapter *WindowsPodmanAdapter) stage(
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

func windowsRuntimeSpec(spec WorkloadSpec) (WorkloadSpec, error) {
	wslDataPath, err := WindowsPathToWSL(spec.DataPath)
	if err != nil {
		return WorkloadSpec{}, err
	}
	spec.DataPath = wslDataPath
	spec.DBHostAddress = "127.0.0.1"
	if err := spec.Validate(); err != nil {
		return WorkloadSpec{}, err
	}

	return spec, nil
}

func commandExitCode(err error) int {
	var withExitCode interface{ ExitCode() int }
	if errors.As(err, &withExitCode) {
		return withExitCode.ExitCode()
	}

	return -1
}

func reportsWSL2(output []byte) bool {
	normalized := strings.ToLower(strings.ReplaceAll(string(output), "\x00", ""))
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ": 2") || strings.HasSuffix(line, ":\t2") {
			return true
		}
	}

	return false
}

func WindowsPathToWSL(path string) (string, error) {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("UNC path %q is not visible through the WSL drive mount", path)
	}
	matches := windowsDrivePath.FindStringSubmatch(path)
	if matches == nil {
		return "", fmt.Errorf("path %q must be an absolute Windows drive path", path)
	}
	drive := strings.ToLower(matches[1])
	remainder := strings.ReplaceAll(matches[2], `\`, "/")
	if remainder != "" &&
		(strings.HasPrefix(remainder, "/") ||
			strings.HasSuffix(remainder, "/") ||
			strings.Contains(remainder, "//")) {
		return "", fmt.Errorf("path %q is not canonical", path)
	}
	parts := strings.Split(remainder, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "":
			if remainder == "" {
				continue
			}

			return "", fmt.Errorf("path %q is not canonical", path)
		case ".", "..":
			return "", fmt.Errorf("path %q contains traversal", path)
		default:
			cleanParts = append(cleanParts, part)
		}
	}

	return "/mnt/" + drive + "/" + strings.Join(cleanParts, "/"), nil
}
