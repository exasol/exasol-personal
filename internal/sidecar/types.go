// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package sidecar

const (
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
