// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

import (
	"slices"
	"strings"
)

const (
	// DatabaseAlias is the name the database answers to on a deployment's
	// service network, and DatabasePort is where it listens there. Neither
	// depends on how the deployment publishes the database to its host.
	DatabaseAlias = "database"
	DatabasePort  = 8563

	Always       = "Always"
	IfNotPresent = "IfNotPresent"
	Never        = "Never"
	OnFailure    = "OnFailure"
)

type Document struct {
	Version    int         `yaml:"version"`
	Containers []Container `yaml:"containers"`
}

type Catalog struct {
	Version  int              `yaml:"version"`
	Sidecars map[string]Entry `yaml:"sidecars"`
}

type Entry struct {
	Description   string    `yaml:"description"`
	Architectures []string  `yaml:"architectures"`
	Container     Container `yaml:"container"`
}

type Container struct {
	Name            string   `yaml:"name"`
	Image           string   `yaml:"image"`
	ImagePullPolicy string   `yaml:"imagePullPolicy,omitempty"`
	Command         []string `yaml:"command,omitempty"`
	Args            []string `yaml:"args,omitempty"`
	WorkingDir      string   `yaml:"workingDir,omitempty"`
	Env             []EnvVar `yaml:"env,omitempty"`
	Ports           []Port   `yaml:"ports,omitempty"`
	RestartPolicy   string   `yaml:"restartPolicy,omitempty"`
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

// PublishedPort is a sidecar endpoint the deployment opens beyond the host
// that runs the container. RuntimePort is where the container runtime bound
// the port on that host, which differs from HostPort when a VM separates the
// two.
type PublishedPort struct {
	Sidecar     string
	Index       int
	HostIP      string
	HostPort    int
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

func (entry Entry) Clone() Entry {
	entry.Architectures = slices.Clone(entry.Architectures)
	entry.Container = entry.Container.Clone()

	return entry
}

func (container Container) Clone() Container {
	container.Command = slices.Clone(container.Command)
	container.Args = slices.Clone(container.Args)
	container.Ports = slices.Clone(container.Ports)
	container.Env = slices.Clone(container.Env)
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
		container.RestartPolicy = OnFailure
	}
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
	}

	return container
}
