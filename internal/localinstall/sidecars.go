// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"go.yaml.in/yaml/v3"
)

const (
	podmanCommand          = "podman"
	labelFlag              = "--label"
	sidecarOwnerLabel      = "com.exasol.launcher.sidecar-owner"
	SidecarNameLabel       = "com.exasol.launcher.sidecar-name"
	sidecarDefinitionLabel = "com.exasol.launcher.sidecar-definition"
	sidecarPortsLabel      = "com.exasol.launcher.sidecar-ports"
)

var runtimeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type SidecarRuntime struct {
	environment  CommandRunner
	owner        string
	PublishPort  func(sidecar.Port) string
	DatabaseHost string
}

type SidecarManager interface {
	List(ctx context.Context) ([]SidecarContainer, error)
	Ensure(ctx context.Context, definition sidecar.Container, sources sidecar.Sources) error
	Remove(ctx context.Context, name string) error
	AttachDatabase(ctx context.Context) (bool, error)
	DatabaseRestartRequired(ctx context.Context) (bool, error)
}

type SidecarProvider interface {
	Sidecars(ctx context.Context) (SidecarManager, error)
	SidecarHostRunning(ctx context.Context) (bool, error)
}

type SidecarContainer struct {
	ID       string            `json:"id"`
	Names    []string          `json:"names"`
	Labels   map[string]string `json:"labels"`
	State    string            `json:"state"`
	ExitCode int               `json:"exitCode"`
	Ports    []SidecarPort     `json:"ports"`
}

//nolint:tagliatelle // Podman's container-list schema uses snake_case.
type SidecarPort struct {
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

func (container SidecarContainer) Name() string  { return container.Labels[SidecarNameLabel] }
func (container SidecarContainer) Running() bool { return container.State == "running" }

func (container SidecarContainer) Matches(definition sidecar.Container) (bool, error) {
	fingerprint, err := SidecarDefinitionHash(definition)
	if err != nil || !container.Running() || container.Name() != definition.Name ||
		container.Labels[sidecarDefinitionLabel] != fingerprint {
		return false, err
	}
	ports := map[SidecarPort]int{}
	for _, port := range definition.Defaults().Ports {
		if port.HostPort > 0 {
			ports[SidecarPort{
				HostIP: port.HostIP, HostPort: port.HostPort,
				ContainerPort: port.ContainerPort, Protocol: strings.ToLower(port.Protocol),
			}]++
		}
	}
	for _, port := range container.Ports {
		if port.HostPort > 0 {
			ports[port]--
		}
	}
	for _, count := range ports {
		if count != 0 {
			return false, nil
		}
	}

	return true, nil
}

func NewSidecarRuntime(environment CommandRunner, owner string) (*SidecarRuntime, error) {
	if environment == nil || !runtimeNamePattern.MatchString(owner) {
		return nil, errors.New(
			"sidecar runtime requires an environment and a valid deployment owner",
		)
	}

	return &SidecarRuntime{environment: environment, owner: owner}, nil
}

func (runtime *SidecarRuntime) List(ctx context.Context) ([]SidecarContainer, error) {
	output, err := runtime.command(ctx, "ps", "--all", "--filter",
		"label="+sidecarOwnerLabel+"="+runtime.owner, "--format", "json")
	if err != nil {
		return nil, err
	}
	var containers []SidecarContainer
	if err := json.Unmarshal(output, &containers); err != nil {
		return nil, fmt.Errorf("decode sidecar container listing: %w", err)
	}
	for index := range containers {
		container := &containers[index]
		if ports := container.Labels[sidecarPortsLabel]; ports != "" {
			if err := json.Unmarshal([]byte(ports), &container.Ports); err != nil {
				return nil, fmt.Errorf("decode sidecar %s endpoints: %w", container.Name(), err)
			}
		}
	}

	return slices.DeleteFunc(containers, func(container SidecarContainer) bool {
		return container.Labels[sidecarOwnerLabel] != runtime.owner || container.Name() == ""
	}), nil
}

func (runtime *SidecarRuntime) Ensure(
	ctx context.Context, definition sidecar.Container, sources sidecar.Sources,
) error {
	resolved, err := sidecar.Resolve(definition, sources)
	if err != nil {
		return err
	}
	fingerprint, err := SidecarDefinitionHash(definition)
	if err != nil {
		return err
	}
	args, err := runtime.createArgs(resolved.Container, fingerprint)
	if err != nil {
		return err
	}
	containers, err := runtime.List(ctx)
	if err != nil {
		return fmt.Errorf("sidecar %s: %w", definition.Name, err)
	}
	for _, container := range containers {
		if container.Name() != definition.Name {
			continue
		}
		if container.Running() && container.Labels[sidecarDefinitionLabel] == fingerprint {
			return nil
		}
		if err := runtime.removeContainer(ctx, container); err != nil {
			return err
		}
	}
	env := make(map[string]string, len(resolved.Container.Env))
	for _, variable := range resolved.Container.Env {
		env[variable.Name] = *variable.Value
	}
	var diagnostic bytes.Buffer
	if err := runtime.environment.Run(ctx, env, nil, io.Discard, &diagnostic, args...); err != nil {
		return fmt.Errorf(
			"sidecar %s: %s",
			definition.Name,
			resolved.Redact(
				fmt.Sprintf("container startup failed: %v: %s", err, diagnostic.String()),
			),
		)
	}

	return nil
}

func (runtime *SidecarRuntime) Remove(ctx context.Context, name string) error {
	containers, err := runtime.List(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, container := range containers {
		if container.Name() == name {
			if err := runtime.removeContainer(ctx, container); err != nil {
				failures = append(failures, err)
			}
		}
	}

	return errors.Join(failures...)
}

func (runtime *SidecarRuntime) createArgs(
	container sidecar.Container,
	fingerprint string,
) ([]string, error) {
	pulls := map[string]string{
		sidecar.Always:       "always",
		sidecar.IfNotPresent: "missing",
		sidecar.Never:        "never",
	}
	restarts := map[string]string{
		sidecar.Always:    "always",
		sidecar.OnFailure: "on-failure",
		sidecar.Never:     "no",
	}
	args := []string{
		podmanCommand, "run", "--detach", "--name", runtime.owner + "-sidecar-" + container.Name,
		labelFlag, sidecarOwnerLabel + "=" + runtime.owner,
		labelFlag, SidecarNameLabel + "=" + container.Name,
		labelFlag, sidecarDefinitionLabel + "=" + fingerprint,
		"--pull", pulls[container.ImagePullPolicy], "--restart", restarts[container.RestartPolicy],
	}
	if runtime.DatabaseHost != "" {
		args = append(args, "--network", "host", "--add-host", "database:"+runtime.DatabaseHost)
		ports := []SidecarPort{}
		for _, port := range container.Ports {
			if port.HostPort > 0 {
				ports = append(ports, SidecarPort{
					HostIP: port.HostIP, HostPort: port.HostPort,
					ContainerPort: port.ContainerPort, Protocol: "tcp",
				})
			}
		}
		encoded, err := json.Marshal(ports)
		if err != nil {
			return nil, err
		}
		args = append(args, labelFlag, sidecarPortsLabel+"="+string(encoded))
	} else {
		args = append(args, "--network", runtime.NetworkName(), "--network-alias", container.Name)
	}
	for _, port := range container.Ports {
		if runtime.DatabaseHost != "" {
			if port.HostPort > 0 && port.HostPort != port.ContainerPort {
				return nil, fmt.Errorf(
					"host networking requires hostPort to equal containerPort for sidecar %s",
					container.Name,
				)
			}

			continue
		}
		if port.HostPort > 0 {
			mapping := net.JoinHostPort(port.HostIP, strconv.Itoa(port.HostPort)) +
				":" + strconv.Itoa(port.ContainerPort) + "/tcp"
			if runtime.PublishPort != nil {
				mapping = runtime.PublishPort(port)
			}
			args = append(args, "--publish", mapping)
		}
	}
	if len(container.Command) > 0 {
		entrypoint, err := json.Marshal(container.Command)
		if err != nil {
			return nil, err
		}
		args = append(args, "--entrypoint", string(entrypoint))
	}
	if container.WorkingDir != "" {
		args = append(args, "--workdir", container.WorkingDir)
	}
	for _, env := range container.Env {
		args = append(args, "--env", env.Name)
	}
	args = append(args, container.Image)
	args = append(args, container.Args...)

	return args, nil
}

func SidecarDefinitionHash(container sidecar.Container) (string, error) {
	data, err := yaml.Marshal(container.Defaults())
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:]), nil
}

func (runtime *SidecarRuntime) removeContainer(
	ctx context.Context,
	container SidecarContainer,
) error {
	if container.ID == "" {
		return errors.New("sidecar runtime returned an empty container ID")
	}
	_, err := runtime.command(ctx, "rm", "--force", "--ignore", "--", container.ID)
	if err != nil {
		return fmt.Errorf("sidecar %s: %w", container.Name(), err)
	}

	return nil
}

func (runtime *SidecarRuntime) command(ctx context.Context, args ...string) ([]byte, error) {
	var output, diagnostic bytes.Buffer
	if err := runtime.environment.Run(ctx, nil, nil, &output, &diagnostic,
		append([]string{podmanCommand}, args...)...); err != nil {
		return nil, fmt.Errorf("podman %s failed: %w: %s", args[0], err, diagnostic.String())
	}

	return output.Bytes(), nil
}
