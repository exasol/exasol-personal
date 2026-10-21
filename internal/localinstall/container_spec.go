// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

// Each runtime spells these its own way.
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

type PortMapping struct {
	// An empty HostIP lets the runtime choose, and a zero HostPort publishes
	// nothing.
	HostIP        string
	HostPort      int
	ContainerPort int
	Protocol      string
	// Mapping overrides the encoded form where a runtime reaches the binding
	// differently than the host that created it.
	Mapping string
}

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
	// The database image depends on both.
	SharedMemorySize string
	ProcessLimit     string
	SecurityOptions  []string
}
