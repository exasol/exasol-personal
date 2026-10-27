// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"io"
	"time"
)

// ContainerRuntime holds every container operation the launcher performs, so
// no caller above it carries a backend's vocabulary.
//
//nolint:interfacebloat // The surface is as wide as the launcher's use.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec, out, outErr io.Writer) error
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

type ContainerFilter struct {
	Name  string
	Label string
}

type ContainerListing struct {
	ID        string
	Names     []string
	Labels    map[string]string
	State     string
	ExitCode  int
	Ports     []ContainerPort
	StartedAt time.Time
}

func (container ContainerListing) Running() bool { return container.State == "running" }

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
