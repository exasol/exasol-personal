// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	DatabaseAlias       = "database"
	DatabasePort        = 8563
	DatabaseSecret      = "exasol-database"
	DatabasePasswordKey = "password"

	Always       = "Always"
	IfNotPresent = "IfNotPresent"
	Never        = "Never"
	OnFailure    = "OnFailure"
)

type SecretValue struct {
	Value string
	// Sensitive keeps the value out of the diagnostics the launcher reports.
	Sensitive bool
}

type Secret map[string]SecretValue

type Secrets map[string]Secret

type Document struct {
	Version  int                   `yaml:"version"`
	Sidecars map[string]Definition `yaml:"sidecars"`
}

type Catalog struct {
	Version  int              `yaml:"version"`
	Sidecars map[string]Entry `yaml:"sidecars"`
}

type Entry struct {
	Definition `yaml:",inline"`

	Description   string   `yaml:"description"`
	Architectures []string `yaml:"architectures"`
}

// Definition is what the catalog offers and what enabling saves, so both files
// describe a sidecar the same way.
type Definition struct {
	// Absent counts as enabled, so a hand-written entry runs without it.
	Enabled   *bool      `yaml:"enabled,omitempty"`
	Container Container  `yaml:"container"`
	Endpoints []Endpoint `yaml:"endpoints,omitempty"`
	// Volumes sit beside the container, as they do in a Kubernetes pod.
	Volumes []Volume `yaml:"volumes,omitempty"`
}

type Volume struct {
	Name   string        `yaml:"name"`
	Secret *SecretVolume `yaml:"secret"`
}

// SecretVolume projects keys of a launcher-owned source into files, so the
// values stay out of the container's inspectable configuration.
type SecretVolume struct {
	SecretName string      `yaml:"secretName"`
	Items      []KeyToPath `yaml:"items,omitempty"`
	Optional   bool        `yaml:"optional,omitempty"`
}

type KeyToPath struct {
	Key  string `yaml:"key"`
	Path string `yaml:"path"`
}

type VolumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	ReadOnly  bool   `yaml:"readOnly,omitempty"`
}

func (definition Definition) IsEnabled() bool {
	return definition.Enabled == nil || *definition.Enabled
}

// Endpoint supplies the scheme and path a container port does not carry, so
// status can report a URL a user can paste into a client.
type Endpoint struct {
	Port   string `yaml:"port"`
	Scheme string `yaml:"scheme"`
	Path   string `yaml:"path,omitempty"`
}

type Container struct {
	Name            string        `yaml:"name"`
	Image           string        `yaml:"image"`
	ImagePullPolicy string        `yaml:"imagePullPolicy,omitempty"`
	Command         []string      `yaml:"command,omitempty"`
	Args            []string      `yaml:"args,omitempty"`
	WorkingDir      string        `yaml:"workingDir,omitempty"`
	Env             []EnvVar      `yaml:"env,omitempty"`
	Ports           []Port        `yaml:"ports,omitempty"`
	RestartPolicy   string        `yaml:"restartPolicy,omitempty"`
	ReadinessProbe  *Probe        `yaml:"readinessProbe,omitempty"`
	VolumeMounts    []VolumeMount `yaml:"volumeMounts,omitempty"`
}

type EnvVar struct {
	Name      string     `yaml:"name"`
	Value     *string    `yaml:"value,omitempty"`
	ValueFrom *EnvSource `yaml:"valueFrom,omitempty"`
}

type EnvSource struct {
	SecretKeyRef *SecretKeyRef `yaml:"secretKeyRef"`
}

type SecretKeyRef struct {
	Name     string `yaml:"name"`
	Key      string `yaml:"key"`
	Optional bool   `yaml:"optional,omitempty"`
}

type PublishedPort struct {
	Sidecar  string
	Index    int
	Name     string
	HostIP   string
	HostPort int
	// Where the container runtime bound the port, which differs from HostPort
	// when a virtual machine separates the two.
	RuntimePort int
}

type Port struct {
	Name          string `yaml:"name,omitempty"`
	ContainerPort int    `yaml:"containerPort"`
	HostPort      int    `yaml:"hostPort,omitempty"`
	//nolint:tagliatelle // Kubernetes uses hostIP.
	HostIP   string `yaml:"hostIP,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`
}

func decode(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	// A typo in a hand-edited document is a mistake, not a future key.
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("document: %w", err)
	}

	return nil
}

func Parse(data []byte) (Document, error) {
	var document Document
	if err := decode(data, &document); err != nil {
		return Document{}, err
	}
	if document.Version != 1 || document.Sidecars == nil {
		return Document{}, errors.New("document: version must be 1 and sidecars must be present")
	}
	for _, name := range document.Names() {
		definition, err := document.Sidecars[name].named(name)
		if err != nil {
			return Document{}, err
		}
		if err := definition.validate(name); err != nil {
			return Document{}, err
		}
		document.Sidecars[name] = definition
	}

	return document, nil
}

// Names orders the saved sidecars, because the map holding them does not.
func (document Document) Names() []string {
	return slices.Sorted(maps.Keys(document.Sidecars))
}

// Desired lists the enabled sidecars, which are the ones that should run.
func (document Document) Desired() []Definition {
	definitions := make([]Definition, 0, len(document.Sidecars))
	for _, name := range document.Names() {
		if definition := document.Sidecars[name]; definition.IsEnabled() {
			definitions = append(definitions, definition)
		}
	}

	return definitions
}

func (entry Entry) Clone() Entry {
	entry.Architectures = slices.Clone(entry.Architectures)
	entry.Definition = entry.Definition.Clone()

	return entry
}

func (definition Definition) Clone() Definition {
	if definition.Enabled != nil {
		enabled := *definition.Enabled
		definition.Enabled = &enabled
	}
	definition.Container = definition.Container.Clone()
	definition.Endpoints = slices.Clone(definition.Endpoints)
	definition.Volumes = slices.Clone(definition.Volumes)
	for index, volume := range definition.Volumes {
		if volume.Secret != nil {
			secret := *volume.Secret
			secret.Items = slices.Clone(secret.Items)
			definition.Volumes[index].Secret = &secret
		}
	}

	return definition
}

func (container Container) Clone() Container {
	container.Command = slices.Clone(container.Command)
	container.Args = slices.Clone(container.Args)
	container.Ports = slices.Clone(container.Ports)
	container.Env = slices.Clone(container.Env)
	container.VolumeMounts = slices.Clone(container.VolumeMounts)
	container.ReadinessProbe = container.ReadinessProbe.clone()
	for index, env := range container.Env {
		if env.Value != nil {
			value := *env.Value
			container.Env[index].Value = &value
		}
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			reference := *env.ValueFrom.SecretKeyRef
			container.Env[index].ValueFrom = &EnvSource{SecretKeyRef: &reference}
		}
	}

	return container
}

func (container Container) Defaults() Container {
	container = container.Clone()
	if container.RestartPolicy == "" {
		container.RestartPolicy = Always
	}
	container.ReadinessProbe.defaults()
	if container.ImagePullPolicy == "" {
		container.ImagePullPolicy = IfNotPresent
		image := container.Image
		if !strings.Contains(image, "@") {
			last := image[strings.LastIndex(image, "/")+1:]
			if !strings.Contains(last, ":") || strings.HasSuffix(last, ":latest") {
				container.ImagePullPolicy = Always
			}
		}
	}
	for index := range container.Ports {
		port := &container.Ports[index]
		if port.Protocol == "" {
			port.Protocol = "TCP"
		}
		if port.HostPort > 0 && port.HostIP == "" {
			port.HostIP = "127.0.0.1"
		}
		// A host IP serves as a map key, so it is normalized before comparison.
		if address, err := netip.ParseAddr(port.HostIP); err == nil {
			port.HostIP = address.String()
		}
	}

	return container
}
