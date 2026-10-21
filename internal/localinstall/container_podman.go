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
	"maps"
	"net"
	"os/exec"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	podmanBinary = "podman"
	// podmanBridgeDriver is how this runtime names a network that routes
	// between containers on one host.
	podmanBridgeDriver = "bridge"
	formatFlag         = "--format"
	filterFlag         = "--filter"
)

type podmanRuntime struct {
	// Decides which host the container runtime runs on.
	environment CommandRunner
	binary      string
}

func newPodmanRuntime(environment CommandRunner) ContainerRuntime {
	return podmanRuntime{environment: environment, binary: podmanBinary}
}

func (runtime podmanRuntime) Create(
	ctx context.Context, spec ContainerSpec, out, outErr io.Writer,
) error {
	args, err := podmanCreateArgs(spec)
	if err != nil {
		return err
	}

	return runtime.run(ctx, out, outErr, args...)
}

func (runtime podmanRuntime) Remove(
	ctx context.Context, name string, force bool, out, outErr io.Writer,
) error {
	if force {
		// Forcing also stops a running container and accepts an absent one.
		return runtime.run(ctx, out, outErr, "rm", "--force", "--ignore", "--", name)
	}

	return runtime.run(ctx, out, outErr, "rm", "--", name)
}

func (runtime podmanRuntime) Stop(ctx context.Context, name string, out, outErr io.Writer) error {
	return runtime.run(ctx, out, outErr, "stop", name)
}

func (runtime podmanRuntime) Rename(
	ctx context.Context, from, to string, out, outErr io.Writer,
) error {
	return runtime.run(ctx, out, outErr, "rename", from, to)
}

func (runtime podmanRuntime) CopyFrom(
	ctx context.Context, container, path, destination string, out, outErr io.Writer,
) error {
	return runtime.run(ctx, out, outErr, "cp", container+":"+path, destination)
}

func (runtime podmanRuntime) List(
	ctx context.Context, outErr io.Writer, filters ...ContainerFilter,
) ([]ContainerListing, error) {
	const filterArguments = 2
	args := make([]string, 0, filterArguments+filterArguments*len(filters))
	args = append(args, "ps", "--all")
	for _, filter := range filters {
		switch {
		case filter.Name != "":
			args = append(args, filterFlag, "name=^"+filter.Name+"$")
		case filter.Label != "":
			args = append(args, filterFlag, "label="+filter.Label)
		default:
		}
	}
	output, err := runtime.output(ctx, nil, outErr, append(args, formatFlag, "json")...)
	if err != nil {
		return nil, err
	}
	// A runtime that reports nothing at all lists no containers.
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	var listings []podmanListing
	if err := json.Unmarshal([]byte(output), &listings); err != nil {
		return nil, fmt.Errorf("decode container listing: %w", err)
	}
	containers := make([]ContainerListing, 0, len(listings))
	for _, listing := range listings {
		containers = append(containers, listing.container())
	}

	return containers, nil
}

func (runtime podmanRuntime) Attachment(
	ctx context.Context, name string, outErr io.Writer,
) (ContainerAttachment, bool, error) {
	present, err := runtime.exists(ctx, name, outErr)
	if err != nil || !present {
		return ContainerAttachment{}, false, err
	}
	output, err := runtime.output(ctx, nil, outErr, "container", "inspect", name)
	if err != nil {
		return ContainerAttachment{}, false, err
	}
	var inspected []podmanInspection
	if err := json.Unmarshal([]byte(output), &inspected); err != nil {
		return ContainerAttachment{}, false, fmt.Errorf("decode container attachment: %w", err)
	}
	if len(inspected) != 1 {
		return ContainerAttachment{}, false, fmt.Errorf("runtime did not report container %s", name)
	}

	return inspected[0].attachment(), true, nil
}

func (runtime podmanRuntime) Mounts(
	ctx context.Context, name string, outErr io.Writer,
) ([]ContainerMount, error) {
	output, err := runtime.output(
		ctx, nil, outErr, "container", "inspect", formatFlag, "{{json .Mounts}}", name,
	)
	if err != nil {
		return nil, err
	}
	var reported []podmanMount
	if err := json.Unmarshal([]byte(output), &reported); err != nil {
		return nil, fmt.Errorf("decode container mounts: %w", err)
	}
	mounts := make([]ContainerMount, 0, len(reported))
	for _, mount := range reported {
		mounts = append(mounts, mount.mount())
	}

	return mounts, nil
}

func (runtime podmanRuntime) Diagnostics(ctx context.Context, container string, out io.Writer) {
	for _, command := range [][]string{
		{"info"},
		{"ps", "-a"},
		{"container", "inspect", container},
		{"logs", container},
	} {
		// Each command is labelled so the reader knows what produced the output.
		_, _ = fmt.Fprintf(out, "$ %s %s\n", runtime.binary, strings.Join(command, " "))
		if err := runtime.run(ctx, out, out, command...); err != nil {
			_, _ = fmt.Fprintf(out, "diagnostic command failed: %v\n", err)
		}
	}
}

func (runtime podmanRuntime) LoadImage(
	ctx context.Context, path string, out, outErr io.Writer,
) (string, error) {
	output, err := runtime.output(ctx, out, outErr, "load", "-i", path)
	if err != nil {
		return "", err
	}

	return parseLoadedImage(output)
}

func (runtime podmanRuntime) ImageExists(
	ctx context.Context, image string, outErr io.Writer,
) (bool, error) {
	return runtime.succeeded(ctx, nil, outErr, "image", "exists", image)
}

func (runtime podmanRuntime) PullImage(
	ctx context.Context, image string, out, outErr io.Writer,
) error {
	return runtime.run(ctx, out, outErr, "pull", image)
}

func (runtime podmanRuntime) RemoveImage(
	ctx context.Context, image string, out, outErr io.Writer,
) error {
	return runtime.run(ctx, out, outErr, "rmi", image)
}

func (runtime podmanRuntime) ImportImage(
	ctx context.Context, path, image string, labels []string, out, outErr io.Writer,
) error {
	args := []string{"import"}
	for _, label := range labels {
		// The labels are what finds these images again.
		args = append(args, "--change", "LABEL "+label)
	}

	return runtime.run(ctx, out, outErr, append(args, path, image)...)
}

func (runtime podmanRuntime) Images(
	ctx context.Context, outErr io.Writer, labels ...string,
) ([]string, error) {
	const filterArguments = 2
	args := make([]string, 0, filterArguments+filterArguments*len(labels))
	args = append(args, "images")
	for _, label := range labels {
		args = append(args, filterFlag, "label="+label)
	}
	output, err := runtime.output(
		ctx, nil, outErr, append(args, formatFlag, "{{.Repository}}:{{.Tag}}")...,
	)
	if err != nil {
		return nil, err
	}

	return strings.Fields(output), nil
}

func (runtime podmanRuntime) CreateNetwork(
	ctx context.Context, spec NetworkSpec, out, outErr io.Writer,
) error {
	// An existing network is accepted rather than reported.
	args := []string{"network", "create", "--ignore", "--driver", podmanBridgeDriver}
	if !spec.DNS {
		args = append(args, "--disable-dns")
	}
	for _, key := range slices.Sorted(maps.Keys(spec.Labels)) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}

	return runtime.run(ctx, out, outErr, append(args, spec.Name)...)
}

func (runtime podmanRuntime) Network(
	ctx context.Context, name string, outErr io.Writer,
) (ContainerNetwork, bool, error) {
	output, err := runtime.output(ctx, nil, outErr, "network", "inspect", name)
	if err != nil {
		return ContainerNetwork{}, false, err
	}
	networks, err := decodePodmanNetworks(output)
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

func (runtime podmanRuntime) Networks(
	ctx context.Context, outErr io.Writer,
) ([]ContainerNetwork, error) {
	output, err := runtime.output(ctx, nil, outErr, "network", "ls", formatFlag, "json")
	if err != nil {
		return nil, err
	}

	return decodePodmanNetworks(output)
}

func (runtime podmanRuntime) ConnectNetwork(
	ctx context.Context, network, container string, aliases []string, out, outErr io.Writer,
) error {
	args := []string{"network", "connect"}
	for _, alias := range aliases {
		args = append(args, "--alias", alias)
	}

	return runtime.run(ctx, out, outErr, append(args, network, container)...)
}

func (runtime podmanRuntime) RemoveNetwork(
	ctx context.Context, name string, out, outErr io.Writer,
) error {
	return runtime.run(ctx, out, outErr, "network", "rm", name)
}

// The runtime matches a name filter loosely, so the exact name decides.
func (runtime podmanRuntime) exists(
	ctx context.Context, name string, outErr io.Writer,
) (bool, error) {
	containers, err := runtime.List(ctx, outErr, ContainerFilter{Name: name})
	if err != nil {
		return false, err
	}

	return slices.ContainsFunc(containers, func(container ContainerListing) bool {
		return slices.Contains(container.Names, name)
	}), nil
}

func (runtime podmanRuntime) run(
	ctx context.Context, out, outErr io.Writer, args ...string,
) error {
	return runtime.environment.Run(
		ctx, nil, out, outErr, append([]string{runtime.binary}, args...)...,
	)
}

func (runtime podmanRuntime) output(
	ctx context.Context, out, outErr io.Writer, args ...string,
) (string, error) {
	var captured bytes.Buffer
	if out == nil {
		out = &captured
	} else {
		out = io.MultiWriter(out, &captured)
	}
	err := runtime.environment.Run(
		ctx, nil, out, outErr, append([]string{runtime.binary}, args...)...,
	)

	return captured.String(), err
}

// succeeded reports a query that answers through its exit status, where exit
// code 1 is the negative answer rather than a failure.
func (runtime podmanRuntime) succeeded(
	ctx context.Context, out, outErr io.Writer, args ...string,
) (bool, error) {
	err := runtime.run(ctx, out, outErr, args...)
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// podmanPullPolicy reports podman's spelling, false where it has none.
func podmanPullPolicy(policy PullPolicy) (string, bool) {
	switch policy {
	case PullAlways:
		return "always", true
	case PullIfNotPresent:
		return "missing", true
	case PullNever:
		return "never", true
	default:
		return "", false
	}
}

// podmanRestartPolicy reports podman's spelling, false where it has none.
func podmanRestartPolicy(policy RestartPolicy) (string, bool) {
	switch policy {
	case RestartAlways:
		return "always", true
	case RestartOnFailure:
		return "on-failure", true
	case RestartNever:
		return "no", true
	default:
		return "", false
	}
}

func podmanCreateArgs(spec ContainerSpec) ([]string, error) {
	args := []string{"run", "--detach"}
	args = podmanIdentityArgs(args, spec)
	args = podmanResourceArgs(args, spec)
	args = podmanNetworkArgs(args, spec)
	args = podmanStorageArgs(args, spec)
	args, err := podmanExecutionArgs(args, spec)
	if err != nil {
		return nil, err
	}

	return append(append(args, spec.Image), spec.Args...), nil
}

func podmanIdentityArgs(args []string, spec ContainerSpec) []string {
	if spec.Replace {
		args = append(args, "--replace")
	}
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	for _, key := range slices.Sorted(maps.Keys(spec.Labels)) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}
	if policy, named := podmanPullPolicy(spec.Pull); named {
		args = append(args, "--pull", policy)
	}
	if policy, named := podmanRestartPolicy(spec.Restart); named {
		args = append(args, "--restart", policy)
	}

	return args
}

func podmanResourceArgs(args []string, spec ContainerSpec) []string {
	if spec.SharedMemorySize != "" {
		args = append(args, "--shm-size="+spec.SharedMemorySize)
	}
	if spec.ProcessLimit != "" && goruntime.GOOS != "windows" {
		args = append(args, "--pids-limit="+spec.ProcessLimit)
	}
	for _, option := range spec.SecurityOptions {
		args = append(args, "--security-opt", option)
	}

	return args
}

func podmanNetworkArgs(args []string, spec ContainerSpec) []string {
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	if spec.NetworkAlias != "" {
		args = append(args, "--network-alias", spec.NetworkAlias)
	}
	for _, port := range spec.Ports {
		args = append(args, "--publish", port.encode())
	}

	return args
}

func podmanStorageArgs(args []string, spec ContainerSpec) []string {
	for _, mount := range spec.Mounts {
		if mount.Kind == MountImage {
			args = append(args, "--mount", mount.encode())

			continue
		}
		args = append(args, "--volume", mount.encode())
	}

	return args
}

func podmanExecutionArgs(args []string, spec ContainerSpec) ([]string, error) {
	for _, key := range slices.Sorted(maps.Keys(spec.Env)) {
		args = append(args, "--env", key+"="+spec.Env[key])
	}
	if spec.EnvFile != "" {
		args = append(args, "--env-file", spec.EnvFile)
	}
	if len(spec.Entrypoint) > 0 {
		entrypoint, err := json.Marshal(spec.Entrypoint)
		if err != nil {
			return nil, err
		}
		args = append(args, "--entrypoint", string(entrypoint))
	}
	if spec.WorkingDir != "" {
		args = append(args, "--workdir", spec.WorkingDir)
	}

	return args, nil
}

// Podman's own reply shapes. Nothing above this file sees them, so another
// runtime maps its wire format to the same vocabulary.
type podmanListing struct {
	ID       string            `json:"id"`
	Names    []string          `json:"names"`
	Labels   map[string]string `json:"labels"`
	State    string            `json:"state"`
	ExitCode int               `json:"exitCode"`
	Ports    []podmanPort      `json:"ports"`
}

//nolint:tagliatelle // Podman's container-list schema uses snake_case.
type podmanPort struct {
	HostIP        string `json:"host_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type podmanNetwork struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	//nolint:tagliatelle // Podman's network schema uses snake_case.
	DNSEnabled bool              `json:"dns_enabled"`
	Labels     map[string]string `json:"labels"`
}

type podmanInspection struct {
	HostConfig      podmanHostConfig      `json:"hostConfig"`
	NetworkSettings podmanNetworkSettings `json:"networkSettings"`
}

type podmanHostConfig struct {
	NetworkMode string `json:"networkMode"`
}

type podmanNetworkSettings struct {
	Networks map[string]podmanNetworkAttachment `json:"networks"`
}

type podmanNetworkAttachment struct {
	Aliases []string `json:"aliases"`
}

//nolint:tagliatelle // Podman inspect emits these exact field names.
type podmanMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

func (listing podmanListing) container() ContainerListing {
	ports := make([]ContainerPort, 0, len(listing.Ports))
	for _, port := range listing.Ports {
		ports = append(ports, ContainerPort(port))
	}

	return ContainerListing{
		ID:       listing.ID,
		Names:    listing.Names,
		Labels:   listing.Labels,
		State:    listing.State,
		ExitCode: listing.ExitCode,
		Ports:    ports,
	}
}

func (network podmanNetwork) network() ContainerNetwork {
	return ContainerNetwork{
		Name:       network.Name,
		Bridge:     network.Driver == podmanBridgeDriver,
		DNSEnabled: network.DNSEnabled,
		Labels:     network.Labels,
	}
}

func (inspection podmanInspection) attachment() ContainerAttachment {
	networks := make(map[string]NetworkMembership, len(inspection.NetworkSettings.Networks))
	for name, membership := range inspection.NetworkSettings.Networks {
		networks[name] = NetworkMembership(membership)
	}

	return ContainerAttachment{
		Bridged:  inspection.HostConfig.NetworkMode == podmanBridgeDriver,
		Networks: networks,
	}
}

func (mount podmanMount) mount() ContainerMount {
	return ContainerMount{
		BindMount:   mount.Type == "bind",
		Source:      mount.Source,
		Destination: mount.Destination,
	}
}

func decodePodmanNetworks(output string) ([]ContainerNetwork, error) {
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	var reported []podmanNetwork
	if err := json.Unmarshal([]byte(output), &reported); err != nil {
		return nil, fmt.Errorf("decode container networks: %w", err)
	}
	networks := make([]ContainerNetwork, 0, len(reported))
	for _, network := range reported {
		networks = append(networks, network.network())
	}

	return networks, nil
}

// parseLoadedImage reads the reference this runtime reports after a load.
func parseLoadedImage(output string) (string, error) {
	var loaded string
	for line := range strings.SplitSeq(output, "\n") {
		_, image, found := strings.Cut(line, loadedImagePrefix)
		if !found {
			continue
		}
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		if loaded != "" && loaded != image {
			return "", errors.New("container runtime reported multiple images")
		}
		loaded = image
	}
	if loaded == "" {
		return "", errors.New("container runtime reported no loaded image")
	}

	return loaded, nil
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
