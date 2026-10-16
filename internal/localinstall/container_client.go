// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// defaultContainerBinary is what local deployments run today. The client
// carries the binary as data so another container runtime can be driven
// through the same commands.
const (
	defaultContainerBinary = "podman"
	// formatFlag selects the runtime's machine-readable output.
	formatFlag = "--format"
)

// ContainerListing is one entry of the container runtime's own listing.
type ContainerListing struct {
	ID       string            `json:"id"`
	Names    []string          `json:"names"`
	Labels   map[string]string `json:"labels"`
	State    string            `json:"state"`
	ExitCode int               `json:"exitCode"`
	Ports    []ContainerPort   `json:"ports"`
}

//nolint:tagliatelle // Podman's container-list schema uses snake_case.
type ContainerPort struct {
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

func (container ContainerListing) Running() bool { return container.State == "running" }

// ContainerNetwork is one entry of the runtime's own network listing.
type ContainerNetwork struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	//nolint:tagliatelle // Podman's network schema uses snake_case.
	DNSEnabled bool              `json:"dns_enabled"`
	Labels     map[string]string `json:"labels"`
}

// Bridge reports a network that routes between containers on one host.
func (network ContainerNetwork) Bridge() bool { return network.Driver == podmanBridgeDriver }

// ContainerAttachment reports how a container is attached to networks.
type ContainerAttachment struct {
	HostConfig      attachmentHostConfig      `json:"hostConfig"`
	NetworkSettings attachmentNetworkSettings `json:"networkSettings"`
}

type attachmentHostConfig struct {
	NetworkMode string `json:"networkMode"`
}

type attachmentNetworkSettings struct {
	Networks map[string]NetworkMembership `json:"networks"`
}

type NetworkMembership struct {
	Aliases []string `json:"aliases"`
}

// Bridged reports a container the runtime can place on a managed network.
func (attachment ContainerAttachment) Bridged() bool {
	return attachment.HostConfig.NetworkMode == podmanBridgeDriver
}

// Member reports the container's membership of one network, if it has one.
func (attachment ContainerAttachment) Member(network string) (NetworkMembership, bool) {
	membership, joined := attachment.NetworkSettings.Networks[network]

	return membership, joined
}

// ContainerMount is one entry of a container's filesystem mounts.
//
//nolint:tagliatelle // Podman inspect emits these exact field names.
type ContainerMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

// BindMount reports a mount that came from a directory on the host.
func (mount ContainerMount) BindMount() bool { return mount.Type == "bind" }

// containerClient runs a container runtime on whichever host an execution
// environment reaches, so container management reads the same whether the
// runtime runs on the user's machine or inside a virtual machine guest.
type containerClient struct {
	environment CommandRunner
	binary      string
}

func newContainerClient(environment CommandRunner) containerClient {
	return containerClient{environment: environment, binary: defaultContainerBinary}
}

// Create starts a container from spec, leaving the runtime's own progress and
// failure output on the writers the caller supplies.
func (client containerClient) Create(
	ctx context.Context, spec ContainerSpec, out, outErr io.Writer,
) error {
	args, err := podmanCreateArgs(spec)
	if err != nil {
		return err
	}

	return client.run(ctx, out, outErr, args...)
}

// Remove deletes a container. Forced removal also stops a running one and
// accepts an absent one, where the plain form reports both as failures.
func (client containerClient) Remove(
	ctx context.Context, name string, force bool, out, outErr io.Writer,
) error {
	if force {
		return client.run(ctx, out, outErr, "rm", "--force", "--ignore", "--", name)
	}

	return client.run(ctx, out, outErr, "rm", "--", name)
}

func (client containerClient) Stop(
	ctx context.Context, name string, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, "stop", name)
}

func (client containerClient) Rename(
	ctx context.Context, from, to string, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, "rename", from, to)
}

// CopyFrom copies a path out of a container onto the host that runs it.
func (client containerClient) CopyFrom(
	ctx context.Context, container, path, destination string, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, "cp", container+":"+path, destination)
}

// Mounts reports where a container's filesystem mounts come from.
func (client containerClient) Mounts(
	ctx context.Context, name string, outErr io.Writer,
) ([]ContainerMount, error) {
	output, err := client.output(
		ctx, nil, outErr, "container", "inspect", formatFlag, "{{json .Mounts}}", name,
	)
	if err != nil {
		return nil, err
	}
	var mounts []ContainerMount
	if err := json.Unmarshal([]byte(output), &mounts); err != nil {
		return nil, fmt.Errorf("decode container mounts: %w", err)
	}

	return mounts, nil
}

func (client containerClient) PullImage(
	ctx context.Context, image string, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, "pull", image)
}

func (client containerClient) RemoveImage(
	ctx context.Context, image string, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, "rmi", image)
}

// ImportImage registers an archive as image, labelled so it can be found again.
func (client containerClient) ImportImage(
	ctx context.Context, path, image string, labels []string, out, outErr io.Writer,
) error {
	args := []string{"import"}
	for _, label := range labels {
		args = append(args, "--change", "LABEL "+label)
	}

	return client.run(ctx, out, outErr, append(args, path, image)...)
}

// Attachment reports how a container is attached to networks, and whether the
// container exists at all. A stopped deployment has none, which callers answer
// from rather than fail on.
func (client containerClient) Attachment(
	ctx context.Context, name string, outErr io.Writer,
) (ContainerAttachment, bool, error) {
	present, err := client.exists(ctx, name, outErr)
	if err != nil || !present {
		return ContainerAttachment{}, false, err
	}
	output, err := client.output(ctx, nil, outErr, "container", "inspect", name)
	if err != nil {
		return ContainerAttachment{}, false, err
	}
	var containers []ContainerAttachment
	if err := json.Unmarshal([]byte(output), &containers); err != nil {
		return ContainerAttachment{}, false, fmt.Errorf("decode container attachment: %w", err)
	}
	if len(containers) != 1 {
		return ContainerAttachment{}, false, fmt.Errorf("runtime did not report container %s", name)
	}

	return containers[0], true, nil
}

// NetworkSpec describes a container network the launcher owns.
type NetworkSpec struct {
	Name   string
	Labels map[string]string
	// DNS lets containers on the network reach each other by name.
	DNS bool
}

// CreateNetwork creates a network, accepting one that already exists.
func (client containerClient) CreateNetwork(
	ctx context.Context, spec NetworkSpec, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, podmanCreateNetworkArgs(spec)...)
}

// Network reports one network the runtime knows, absent when it has none.
func (client containerClient) Network(
	ctx context.Context, name string, outErr io.Writer,
) (ContainerNetwork, bool, error) {
	output, err := client.output(ctx, nil, outErr, "network", "inspect", name)
	if err != nil {
		return ContainerNetwork{}, false, err
	}
	networks, err := decodeNetworks(output)
	if err != nil {
		return ContainerNetwork{}, false, err
	}
	for _, network := range networks {
		if network.Name == name {
			return network, true, nil
		}
	}

	return ContainerNetwork{}, false, nil
}

func (client containerClient) Networks(
	ctx context.Context, outErr io.Writer,
) ([]ContainerNetwork, error) {
	output, err := client.output(ctx, nil, outErr, "network", "ls", "--format", "json")
	if err != nil {
		return nil, err
	}

	return decodeNetworks(output)
}

// ConnectNetwork attaches a container to a network under the given aliases.
func (client containerClient) ConnectNetwork(
	ctx context.Context, network, container string, aliases []string, out, outErr io.Writer,
) error {
	args := []string{"network", "connect"}
	for _, alias := range aliases {
		args = append(args, "--alias", alias)
	}

	return client.run(ctx, out, outErr, append(args, network, container)...)
}

func (client containerClient) RemoveNetwork(
	ctx context.Context, name string, out, outErr io.Writer,
) error {
	return client.run(ctx, out, outErr, "network", "rm", name)
}

// Images reports the runtime's tagged images, narrowed by an optional filter.
func (client containerClient) Images(
	ctx context.Context, outErr io.Writer, filters ...string,
) ([]string, error) {
	args := make([]string, 0, 1+2*len(filters))
	args = append(args, "images")
	for _, filter := range filters {
		args = append(args, "--filter", filter)
	}
	output, err := client.output(
		ctx, nil, outErr, append(args, formatFlag, "{{.Repository}}:{{.Tag}}")...,
	)
	if err != nil {
		return nil, err
	}

	return strings.Fields(output), nil
}

// LoadImage reads an image archive and reports the reference it registered.
func (client containerClient) LoadImage(
	ctx context.Context, path string, out, outErr io.Writer,
) (string, error) {
	output, err := client.output(ctx, out, outErr, "load", "-i", path)
	if err != nil {
		return "", err
	}

	return parseLoadedImage(output)
}

func (client containerClient) ImageExists(
	ctx context.Context, image string, outErr io.Writer,
) (bool, error) {
	return client.succeeded(ctx, nil, outErr, "image", "exists", image)
}

// Diagnostics dumps whatever the runtime can say about a failed container,
// labelling each command so the reader knows what produced the output.
func (client containerClient) Diagnostics(
	ctx context.Context, container string, out io.Writer,
) {
	for _, command := range podmanDiagnosticCommands(container) {
		_, _ = fmt.Fprintf(out, "$ %s %s\n", client.binary, strings.Join(command, " "))
		if err := client.run(ctx, out, out, command...); err != nil {
			_, _ = fmt.Fprintf(out, "diagnostic command failed: %v\n", err)
		}
	}
}

// The runtime matches a name filter loosely, so the exact name decides.
func (client containerClient) exists(
	ctx context.Context, name string, outErr io.Writer,
) (bool, error) {
	containers, err := client.containers(ctx, outErr, "name="+name)
	if err != nil {
		return false, err
	}

	return slices.ContainsFunc(containers, func(container ContainerListing) bool {
		return slices.Contains(container.Names, name)
	}), nil
}

func (client containerClient) run(
	ctx context.Context, out, outErr io.Writer, args ...string,
) error {
	return client.environment.Run(
		ctx, nil, out, outErr, append([]string{client.binary}, args...)...,
	)
}

func (client containerClient) output(
	ctx context.Context, out, outErr io.Writer, args ...string,
) (string, error) {
	var captured bytes.Buffer
	if out == nil {
		out = &captured
	} else {
		out = io.MultiWriter(out, &captured)
	}
	err := client.environment.Run(
		ctx, nil, out, outErr, append([]string{client.binary}, args...)...,
	)

	return captured.String(), err
}

// succeeded reports a query that answers through its exit status, where exit
// code 1 is the negative answer rather than a failure.
func (client containerClient) succeeded(
	ctx context.Context, out, outErr io.Writer, args ...string,
) (bool, error) {
	err := client.run(ctx, out, outErr, args...)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// containers reports the runtime's own container listing for a filter. Both
// the database install and sidecars ask the same question of it, so the filter
// narrows the query and the caller confirms the match it wanted.
func (client containerClient) containers(
	ctx context.Context, outErr io.Writer, filters ...string,
) ([]ContainerListing, error) {
	const filterArguments = 2
	args := make([]string, 0, filterArguments+filterArguments*len(filters))
	args = append(args, "ps", "--all")
	for _, filter := range filters {
		args = append(args, "--filter", filter)
	}
	output, err := client.output(ctx, nil, outErr, append(args, formatFlag, "json")...)
	if err != nil {
		return nil, err
	}
	// A runtime that reports nothing at all lists no containers.
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	var containers []ContainerListing
	if err := json.Unmarshal([]byte(output), &containers); err != nil {
		return nil, fmt.Errorf("decode container listing: %w", err)
	}

	return containers, nil
}
