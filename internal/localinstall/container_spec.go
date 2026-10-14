// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"fmt"
	"net"
	"strconv"
)

// PullPolicy and RestartPolicy name container behaviour the launcher asks for,
// leaving each runtime to spell them its own way.
type (
	PullPolicy    string
	RestartPolicy string
)

const (
	PullAlways       PullPolicy = "always"
	PullIfNotPresent PullPolicy = "if-not-present"
	PullNever        PullPolicy = "never"

	RestartNever     RestartPolicy = "never"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartAlways    RestartPolicy = "always"
)

// PortMapping publishes a container port on the host that runs the container.
// An empty HostIP lets the runtime choose, and a zero HostPort publishes
// nothing.
type PortMapping struct {
	HostIP        string
	HostPort      int
	ContainerPort int
	Protocol      string
	// Mapping overrides the encoded form where a runtime reaches the binding
	// differently than the host that created it.
	Mapping string
}

// MountKind distinguishes a host directory from a directory an image supplies.
type MountKind string

const (
	MountBind  MountKind = "bind"
	MountImage MountKind = "image"
)

type Mount struct {
	Kind     MountKind
	Source   string
	Target   string
	ReadOnly bool
	// Relabel asks the runtime to make the source readable under SELinux.
	Relabel bool
	// Writable keeps an image mount's writes in a per-container overlay.
	Writable bool
}

// ContainerSpec describes a container to create. Callers fill in what they
// need and the client encodes it for whichever runtime it drives.
type ContainerSpec struct {
	Name    string
	Image   string
	Args    []string
	Labels  map[string]string
	Env     map[string]string
	EnvFile string

	Network      string
	NetworkAlias string
	Ports        []PortMapping
	Mounts       []Mount

	Entrypoint []string
	WorkingDir string

	Pull    PullPolicy
	Restart RestartPolicy

	// Replace removes a container of the same name instead of failing.
	Replace bool
	// SharedMemorySize and ProcessLimit are passed through as the runtime
	// spells them, because the database image depends on both.
	SharedMemorySize string
	ProcessLimit     string
	SecurityOptions  []string
}

func (port PortMapping) encode() string {
	if port.Mapping != "" {
		return port.Mapping
	}
	protocol := port.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	published := strconv.Itoa(port.HostPort) + ":" + strconv.Itoa(port.ContainerPort)
	if port.HostIP != "" {
		published = net.JoinHostPort(port.HostIP, strconv.Itoa(port.HostPort)) +
			":" + strconv.Itoa(port.ContainerPort)
	}

	return published + "/" + protocol
}

func (mount Mount) encode() string {
	if mount.Kind == MountImage {
		return fmt.Sprintf(
			"type=image,source=%s,destination=%s,rw=%t",
			mount.Source, mount.Target, mount.Writable,
		)
	}
	options := ""
	if mount.ReadOnly {
		options = ":ro"
	}
	if mount.Relabel {
		options += ",Z"
	}

	return mount.Source + ":" + mount.Target + options
}
