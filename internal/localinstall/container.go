// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// ContainerEngine names the container engine a local deployment runs on.
type ContainerEngine string

const (
	EnginePodman ContainerEngine = "podman"
	EngineDocker ContainerEngine = "docker"
)

var ErrUnsupportedContainerEngine = errors.New("unsupported container engine")

// ContainerRuntime holds every container operation the launcher performs, so
// no caller above it carries a backend's vocabulary.
//
//nolint:interfacebloat // The surface is as wide as the launcher's use.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec, out, outErr io.Writer) error
	// RunToCompletion runs a container in the foreground and removes it after it exits.
	RunToCompletion(ctx context.Context, spec ContainerSpec, out, outErr io.Writer) error
	Remove(ctx context.Context, name string, force bool, out, outErr io.Writer) error
	Stop(ctx context.Context, name string, out, outErr io.Writer) error
	Rename(ctx context.Context, from, to string, out, outErr io.Writer) error
	CopyFrom(ctx context.Context, container, path, destination string, out, outErr io.Writer) error
	Exec(ctx context.Context, container string, command []string, out, outErr io.Writer) error
	List(ctx context.Context, outErr io.Writer, filters ...ContainerFilter) (
		[]ContainerListing, error,
	)
	Attachment(ctx context.Context, name string, outErr io.Writer) (
		ContainerAttachment, bool, error,
	)
	Mounts(ctx context.Context, name string, outErr io.Writer) ([]ContainerMount, error)
	Diagnostics(ctx context.Context, container string, out io.Writer)

	LoadImage(ctx context.Context, path string, out, outErr io.Writer) (string, error)
	ImageExists(ctx context.Context, image string, outErr io.Writer) (bool, error)
	PullImage(ctx context.Context, image string, out, outErr io.Writer) error
	RemoveImage(ctx context.Context, image string, out, outErr io.Writer) error
	ImportImage(
		ctx context.Context, path, image string, labels []string, out, outErr io.Writer,
	) error
	Images(ctx context.Context, outErr io.Writer, labels ...string) ([]string, error)

	CreateNetwork(ctx context.Context, spec NetworkSpec, out, outErr io.Writer) error
	Network(ctx context.Context, name string, outErr io.Writer) (ContainerNetwork, bool, error)
	Networks(ctx context.Context, outErr io.Writer) ([]ContainerNetwork, error)
	ConnectNetwork(
		ctx context.Context, network, container string, aliases []string, out, outErr io.Writer,
	) error
	RemoveNetwork(ctx context.Context, name string, out, outErr io.Writer) error
}

// NewContainerRuntime returns the runtime that drives engine through
// environment.
func NewContainerRuntime(
	engine ContainerEngine,
	environment CommandRunner,
) (ContainerRuntime, error) {
	switch engine {
	case EnginePodman:
		return newPodmanRuntime(environment), nil
	case EngineDocker:
		return newDockerRuntime(environment), nil
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupportedContainerEngine, engine)
	}
}

type ContainerFilter struct {
	Name  string
	Label string
}

type ContainerListing struct {
	ID     string
	Names  []string
	Labels map[string]string
	// State is the engine's own word for the container's state, for reporting.
	State     string
	Lifecycle ContainerLifecycle
	ExitCode  int
	Ports     []ContainerPort
	StartedAt time.Time
}

// ContainerLifecycle is a container's state in terms every engine shares.
type ContainerLifecycle int

const (
	// ContainerLifecycleUnknown is a state the engine adapter does not recognise.
	ContainerLifecycleUnknown ContainerLifecycle = iota
	ContainerLifecycleRunning
	// ContainerLifecycleRestarting waits for its engine to restart it.
	ContainerLifecycleRestarting
	ContainerLifecyclePaused
	// ContainerLifecycleStopped stays down until something starts it.
	ContainerLifecycleStopped
)

func (container ContainerListing) Running() bool {
	return container.Lifecycle == ContainerLifecycleRunning
}

// Active reports a container that its engine keeps alive or can resume without
// the launcher. An unrecognised state counts, so it is never taken for stopped.
func (container ContainerListing) Active() bool {
	return container.Lifecycle != ContainerLifecycleStopped
}

type ContainerPort struct {
	HostIP        string
	HostPort      int
	ContainerPort int
	Protocol      string
}

type ContainerNetwork struct {
	Name       string
	Bridge     bool
	DNSEnabled bool
	Labels     map[string]string
}

type ContainerAttachment struct {
	// Bridged reports a container the runtime can place on a managed network.
	Bridged  bool
	Networks map[string]NetworkMembership
}

type NetworkMembership struct {
	Aliases []string
}

func (attachment ContainerAttachment) Member(network string) (NetworkMembership, bool) {
	membership, joined := attachment.Networks[network]

	return membership, joined
}

type ContainerMount struct {
	BindMount   bool
	Source      string
	Destination string
}

type NetworkSpec struct {
	Name   string
	Labels map[string]string
	// DNS lets containers on the network reach each other by name.
	DNS bool
}
