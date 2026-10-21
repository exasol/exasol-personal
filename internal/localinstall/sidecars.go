// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"go.yaml.in/yaml/v3"
)

const (
	sidecarOwnerLabel      = "com.exasol.launcher.sidecar-owner"
	SidecarNameLabel       = "com.exasol.launcher.sidecar-name"
	sidecarDefinitionLabel = "com.exasol.launcher.sidecar-definition"
)

// How the container runtime reports a claimed address.
var runtimeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type SidecarRuntime struct {
	client      ContainerRuntime
	owner       string
	PublishPort func(sidecar.Port) string
	// Named on both sides of a virtual machine boundary.
	EnvDir RuntimePath
}

type SidecarProvider interface {
	Sidecars(ctx context.Context) (*SidecarRuntime, error)
	SidecarHostRunning(ctx context.Context) (bool, error)
}

func (container ContainerListing) SidecarName() string {
	return container.Labels[SidecarNameLabel]
}

func NewSidecarRuntime(
	environment CommandRunner, owner string, envDir RuntimePath,
) (*SidecarRuntime, error) {
	if environment == nil || !runtimeNamePattern.MatchString(owner) {
		return nil, errors.New(
			"sidecar runtime requires an environment and a valid deployment owner",
		)
	}

	return &SidecarRuntime{
		client: newPodmanRuntime(environment), owner: owner, EnvDir: envDir,
	}, nil
}

func (runtime *SidecarRuntime) ContainerName(sidecarName string) string {
	return runtime.owner + "-sidecar-" + sidecarName
}

func (runtime *SidecarRuntime) Matches(
	container ContainerListing, definition sidecar.Container, secrets sidecar.Secrets,
) (bool, error) {
	// Reachability depends on the host, so published ports stand in for it.
	if !container.Running() || container.SidecarName() != definition.Name {
		return false, nil
	}
	resolved, err := sidecar.Resolve(definition, secrets)
	if err != nil {
		return false, err
	}
	fingerprint, err := runtime.fingerprint(resolved.Container)
	if err != nil {
		return false, err
	}

	return container.Labels[sidecarDefinitionLabel] == fingerprint, nil
}

func (runtime *SidecarRuntime) List(ctx context.Context) ([]ContainerListing, error) {
	containers, err := runtime.client.List(
		ctx, nil, ContainerFilter{Label: sidecarOwnerLabel + "=" + runtime.owner},
	)
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(containers, func(container ContainerListing) bool {
		return container.Labels[sidecarOwnerLabel] != runtime.owner || container.SidecarName() == ""
	}), nil
}

func (runtime *SidecarRuntime) Ensure(
	ctx context.Context, definition sidecar.Container, secrets sidecar.Secrets,
) error {
	resolved, err := sidecar.Resolve(definition, secrets)
	if err != nil {
		return err
	}
	fingerprint, err := runtime.fingerprint(resolved.Container)
	if err != nil {
		return err
	}
	converged, err := runtime.converge(ctx, definition, fingerprint)
	if err != nil || converged {
		return err
	}
	values := make(map[string]string, len(resolved.Container.Env))
	for _, variable := range resolved.Container.Env {
		values[variable.Name] = *variable.Value
	}
	envPath := RuntimePath{}
	remove := noCleanup
	if len(values) > 0 {
		envPath, remove, err = envFile(runtime.EnvDir, definition.Name, values)
		if err != nil {
			return err
		}
	}
	defer remove()
	spec := runtime.createSpec(resolved.Container, fingerprint, envPath.RuntimePath)
	var diagnostic bytes.Buffer
	if err := runtime.client.Create(ctx, spec, io.Discard, &diagnostic); err != nil {
		return sidecar.NewStartupError(
			definition.Name,
			resolved.Redact(diagnostic.String()),
			resolved.Redact(err.Error()),
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
		if container.SidecarName() == name {
			if err := runtime.removeContainer(ctx, container); err != nil {
				failures = append(failures, err)
			}
		}
	}

	return errors.Join(failures...)
}

func (runtime *SidecarRuntime) converge(
	ctx context.Context, definition sidecar.Container, fingerprint string,
) (bool, error) {
	containers, err := runtime.List(ctx)
	if err != nil {
		return false, fmt.Errorf("sidecar %s: %w", definition.Name, err)
	}
	for _, container := range containers {
		if container.SidecarName() != definition.Name {
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

//nolint:gochecknoglobals // Lookup tables, not state.
var (
	sidecarPulls = map[string]PullPolicy{
		sidecar.Always:       PullAlways,
		sidecar.IfNotPresent: PullIfNotPresent,
		sidecar.Never:        PullNever,
	}
	sidecarRestarts = map[string]RestartPolicy{
		sidecar.Always:    RestartAlways,
		sidecar.OnFailure: RestartOnFailure,
		sidecar.Never:     RestartNever,
	}
)

func (runtime *SidecarRuntime) createSpec(
	container sidecar.Container,
	fingerprint string,
	envPath string,
) ContainerSpec {
	spec := ContainerSpec{
		Name:  runtime.ContainerName(container.Name),
		Image: container.Image,
		Args:  container.Args,
		Labels: map[string]string{
			sidecarOwnerLabel:      runtime.owner,
			SidecarNameLabel:       container.Name,
			sidecarDefinitionLabel: fingerprint,
		},
		Pull:         sidecarPulls[container.ImagePullPolicy],
		Restart:      sidecarRestarts[container.RestartPolicy],
		Network:      runtime.NetworkName(),
		NetworkAlias: container.Name,
		Entrypoint:   container.Command,
		WorkingDir:   container.WorkingDir,
		EnvFile:      envPath,
	}
	for _, port := range container.Ports {
		if port.HostPort == 0 {
			continue
		}
		mapping := PortMapping{
			HostIP: port.HostIP, HostPort: port.HostPort, ContainerPort: port.ContainerPort,
		}
		if runtime.PublishPort != nil {
			mapping.Mapping = runtime.PublishPort(port)
		}
		spec.Ports = append(spec.Ports, mapping)
	}

	return spec
}

func (*SidecarRuntime) fingerprint(resolved sidecar.Container) (string, error) {
	data, err := yaml.Marshal(resolved.Defaults())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func (runtime *SidecarRuntime) removeContainer(
	ctx context.Context,
	container ContainerListing,
) error {
	if container.ID == "" {
		return errors.New("sidecar runtime returned an empty container ID")
	}
	if err := runtime.client.Remove(ctx, container.ID, true, nil, nil); err != nil {
		return fmt.Errorf("sidecar %s: %w", container.SidecarName(), err)
	}

	return nil
}
