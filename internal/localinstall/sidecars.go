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

// Matches reports whether the running container applies the definition.
// Where its endpoints end up reachable depends on the deployment host and is
// compared against the deployment's published ports instead.
func (container SidecarContainer) Matches(definition sidecar.Container) (bool, error) {
	fingerprint, err := SidecarDefinitionHash(definition)
	if err != nil || !container.Running() || container.Name() != definition.Name ||
		container.Labels[sidecarDefinitionLabel] != fingerprint {
		return false, err
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

func (runtime *SidecarRuntime) ContainerName(sidecarName string) string {
	return runtime.owner + "-sidecar-" + sidecarName
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

// sidecarPlan carries everything one container creation needs, so transports
// that start containers differently still share resolution and comparison.
type sidecarPlan struct {
	resolved    sidecar.Resolved
	fingerprint string
	env         map[string]string
	args        []string
}

func (runtime *SidecarRuntime) Ensure(
	ctx context.Context, definition sidecar.Container, sources sidecar.Sources,
) error {
	plan, err := runtime.plan(definition, sources, containerCreation{
		detach:  true,
		restart: podmanRestartPolicies[definition.Defaults().RestartPolicy],
		env:     func(name string) []string { return []string{"--env", name} },
	})
	if err != nil {
		return err
	}
	converged, err := runtime.converge(ctx, definition, plan.fingerprint)
	if err != nil || converged {
		return err
	}
	var diagnostic bytes.Buffer
	if err := runtime.environment.Run(
		ctx, plan.env, nil, io.Discard, &diagnostic, plan.args...,
	); err != nil {
		return plan.startupError(definition.Name, err, diagnostic.String())
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

// containerCreation varies how one transport starts a container: a systemd
// service owns process restart and delivers values as container secrets, while
// direct execution leaves both to Podman and the child environment.
type containerCreation struct {
	fingerprint string
	detach      bool
	restart     string
	env         func(name string) []string
}

var podmanRestartPolicies = map[string]string{
	sidecar.Always:    nanoRestartPolicy,
	sidecar.OnFailure: "on-failure",
	sidecar.Never:     "no",
}

func (runtime *SidecarRuntime) plan(
	definition sidecar.Container, sources sidecar.Sources, creation containerCreation,
) (sidecarPlan, error) {
	resolved, err := sidecar.Resolve(definition, sources)
	if err != nil {
		return sidecarPlan{}, err
	}
	fingerprint, err := SidecarDefinitionHash(definition)
	if err != nil {
		return sidecarPlan{}, err
	}
	creation.fingerprint = fingerprint
	args, err := runtime.createArgs(resolved.Container, creation)
	if err != nil {
		return sidecarPlan{}, err
	}
	env := make(map[string]string, len(resolved.Container.Env))
	for _, variable := range resolved.Container.Env {
		env[variable.Name] = *variable.Value
	}

	return sidecarPlan{resolved: resolved, fingerprint: fingerprint, env: env, args: args}, nil
}

// applied reports whether a running container already carries the definition.
func (runtime *SidecarRuntime) applied(
	ctx context.Context, name, fingerprint string,
) (bool, error) {
	containers, err := runtime.List(ctx)
	if err != nil {
		return false, fmt.Errorf("sidecar %s: %w", name, err)
	}
	for _, container := range containers {
		if container.Name() == name && container.Running() &&
			container.Labels[sidecarDefinitionLabel] == fingerprint {
			return true, nil
		}
	}

	return false, nil
}

// converge reports whether the running container already applies the
// definition, removing an outdated one so the caller can create its successor.
func (runtime *SidecarRuntime) converge(
	ctx context.Context, definition sidecar.Container, fingerprint string,
) (bool, error) {
	containers, err := runtime.List(ctx)
	if err != nil {
		return false, fmt.Errorf("sidecar %s: %w", definition.Name, err)
	}
	for _, container := range containers {
		if container.Name() != definition.Name {
			continue
		}
		if container.Running() && container.Labels[sidecarDefinitionLabel] == fingerprint {
			return true, nil
		}
		if err := runtime.removeContainer(ctx, container); err != nil {
			return false, err
		}
	}

	return false, nil
}

func (plan sidecarPlan) startupError(name string, failure error, diagnostic string) error {
	return fmt.Errorf(
		"sidecar %s: %s",
		name,
		plan.resolved.Redact(
			fmt.Sprintf("container startup failed: %v: %s", failure, diagnostic),
		),
	)
}

func (runtime *SidecarRuntime) createArgs(
	container sidecar.Container,
	creation containerCreation,
) ([]string, error) {
	pulls := map[string]string{
		sidecar.Always:       "always",
		sidecar.IfNotPresent: "missing",
		sidecar.Never:        "never",
	}
	args := []string{
		podmanCommand, "run", "--replace", "--name", runtime.ContainerName(container.Name),
		labelFlag, sidecarOwnerLabel + "=" + runtime.owner,
		labelFlag, SidecarNameLabel + "=" + container.Name,
		labelFlag, sidecarDefinitionLabel + "=" + creation.fingerprint,
		"--pull", pulls[container.ImagePullPolicy], "--restart", creation.restart,
	}
	if creation.detach {
		args = append(args, "--detach")
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
		args = append(args, creation.env(env.Name)...)
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
