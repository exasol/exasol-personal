// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	dockerBinary = "docker"
	// dockerDefaultBridge is the network every container joins unless told
	// otherwise. Unlike user-defined bridges it resolves no container names.
	dockerDefaultBridge  = "bridge"
	dockerNetworkCommand = "network"
	dockerInspectCommand = "inspect"
)

var dockerDialect = cliDialect{
	// Docker image mounts are read-only; it has no per-container overlay.
	writableImageMounts: false,
	// Docker's default seccomp and AppArmor profiles block the namespaces the
	// UDF sandbox creates, and the kernel refuses the sandbox a fresh /proc
	// while Docker masks paths below it.
	nestedSandbox: []string{"seccomp=unconfined", "apparmor=unconfined", "systempaths=unconfined"},
}

// userNamespaceRestriction is set where AppArmor confines user namespaces that
// unconfined processes create, as on Ubuntu 24.04.
const userNamespaceRestriction = "/proc/sys/kernel/apparmor_restrict_unprivileged_userns"

// dockerRuntime shares podmanRuntime's commands where the two CLIs agree and
// replaces those whose flags or replies differ.
type dockerRuntime struct {
	podmanRuntime

	// Read on the launcher's host, which is the daemon's: the database
	// container bind-mounts the deployment directory.
	userNamespacesRestricted func() bool
}

func newDockerRuntime(environment CommandRunner) ContainerRuntime {
	return dockerRuntime{podmanRuntime{
		environment: environment, binary: dockerBinary, dialect: dockerDialect,
	}, hostRestrictsUserNamespaces}
}

func hostRestrictsUserNamespaces() bool {
	setting, err := os.ReadFile(userNamespaceRestriction)

	return err == nil && strings.TrimSpace(string(setting)) == "1"
}

func (runtime dockerRuntime) Create(
	ctx context.Context, spec ContainerSpec, out, outErr io.Writer,
) error {
	if spec.Replace && spec.Name != "" {
		if err := runtime.Remove(ctx, spec.Name, true, out, outErr); err != nil {
			return err
		}
	}
	if spec.NestedSandbox {
		privileged, err := runtime.sandboxNeedsSysAdmin(ctx, outErr)
		if err != nil {
			return err
		}
		if privileged {
			spec.Capabilities = append(slices.Clone(spec.Capabilities), "SYS_ADMIN")
		}
	}

	return runtime.podmanRuntime.Create(ctx, spec, out, outErr)
}

func (runtime dockerRuntime) Remove(
	ctx context.Context, name string, force bool, out, outErr io.Writer,
) error {
	if !force {
		return runtime.run(ctx, out, outErr, "rm", "--", name)
	}
	// name may also be an ID, so absence is read from Docker's reply rather
	// than looked up by name first.
	var diagnostic bytes.Buffer
	errOutput := io.Writer(&diagnostic)
	if outErr != nil {
		errOutput = io.MultiWriter(outErr, &diagnostic)
	}
	err := runtime.run(ctx, out, errOutput, "rm", "--force", "--", name)
	if err != nil && strings.Contains(diagnostic.String(), "No such container") {
		return nil
	}

	return err
}

// List inspects the matching containers, because Docker's listing flattens
// labels and ports into display strings and omits exit codes.
func (runtime dockerRuntime) List(
	ctx context.Context, outErr io.Writer, filters ...ContainerFilter,
) ([]ContainerListing, error) {
	args := []string{"ps", "--all", "--quiet", "--no-trunc"}
	for _, filter := range filters {
		switch {
		case filter.Name != "":
			args = append(args, filterFlag, "name=^"+filter.Name+"$")
		case filter.Label != "":
			args = append(args, filterFlag, "label="+filter.Label)
		default:
		}
	}
	output, err := runtime.output(ctx, nil, outErr, args...)
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(output)
	if len(ids) == 0 {
		return nil, nil
	}
	inspected, err := runtime.inspectContainers(ctx, outErr, ids...)
	if err != nil {
		return nil, err
	}
	containers := make([]ContainerListing, 0, len(inspected))
	for _, container := range inspected {
		containers = append(containers, container.listing())
	}

	return containers, nil
}

func (runtime dockerRuntime) Attachment(
	ctx context.Context, name string, outErr io.Writer,
) (ContainerAttachment, bool, error) {
	present, err := runtime.exists(ctx, name, outErr)
	if err != nil || !present {
		return ContainerAttachment{}, false, err
	}
	inspected, err := runtime.inspectContainers(ctx, outErr, name)
	if err != nil || len(inspected) == 0 {
		return ContainerAttachment{}, false, err
	}
	if len(inspected) != 1 {
		return ContainerAttachment{}, false, fmt.Errorf("runtime did not report container %s", name)
	}

	return inspected[0].attachment(), true, nil
}

// ImageExists inspects the image, because Docker's image listing finds nothing
// for a reference that names a digest. Absence is read from Docker's reply.
func (runtime dockerRuntime) ImageExists(
	ctx context.Context, image string, outErr io.Writer,
) (bool, error) {
	var diagnostic bytes.Buffer
	err := runtime.run(
		ctx, nil, &diagnostic, "image", dockerInspectCommand, "--format", "{{.Id}}", "--", image,
	)
	if err == nil {
		return true, nil
	}
	if strings.Contains(diagnostic.String(), "No such image") {
		return false, nil
	}
	if outErr != nil {
		_, _ = outErr.Write(diagnostic.Bytes())
	}

	return false, err
}

func (runtime dockerRuntime) CreateNetwork(
	ctx context.Context, spec NetworkSpec, out, outErr io.Writer,
) error {
	// Docker rejects an existing name instead of accepting it, and every
	// user-defined bridge resolves container names, so spec.DNS needs no flag.
	if _, present, err := runtime.Network(ctx, spec.Name, outErr); err != nil || present {
		return err
	}
	args := []string{dockerNetworkCommand, "create", "--driver", dockerDefaultBridge}
	for _, key := range slices.Sorted(maps.Keys(spec.Labels)) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}

	return runtime.run(ctx, out, outErr, append(args, spec.Name)...)
}

func (runtime dockerRuntime) Network(
	ctx context.Context, name string, outErr io.Writer,
) (ContainerNetwork, bool, error) {
	reported, err := inspectListed[dockerNetwork](
		ctx, runtime, outErr, dockerNetworkCommand, []string{name},
	)
	if err != nil {
		return ContainerNetwork{}, false, err
	}
	for _, network := range reported {
		if network.Name == name {
			return network.network(), true, nil
		}
	}

	return ContainerNetwork{}, false, nil
}

func (runtime dockerRuntime) Networks(
	ctx context.Context, outErr io.Writer,
) ([]ContainerNetwork, error) {
	output, err := runtime.output(
		ctx,
		nil,
		outErr,
		dockerNetworkCommand,
		"ls",
		"--quiet",
		"--no-trunc",
	)
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(output)
	if len(ids) == 0 {
		return nil, nil
	}
	reported, err := inspectListed[dockerNetwork](
		ctx, runtime, outErr, dockerNetworkCommand, ids,
	)
	if err != nil {
		return nil, err
	}
	networks := make([]ContainerNetwork, 0, len(reported))
	for _, network := range reported {
		networks = append(networks, network.network())
	}

	return networks, nil
}

// sandboxNeedsSysAdmin reports whether the UDF sandbox needs SYS_ADMIN from
// the container. It otherwise gains it by creating a user namespace, which
// AppArmor confines under the host's restriction when a rootful daemon applies
// AppArmor to the unconfined container.
func (runtime dockerRuntime) sandboxNeedsSysAdmin(
	ctx context.Context, outErr io.Writer,
) (bool, error) {
	if !runtime.userNamespacesRestricted() {
		return false, nil
	}
	output, err := runtime.output(ctx, nil, outErr, "info", "--format", "{{json .SecurityOptions}}")
	if err != nil {
		return false, err
	}
	var securityOptions []string
	if err := json.Unmarshal([]byte(output), &securityOptions); err != nil {
		return false, fmt.Errorf("decode Docker security options: %w", err)
	}
	apparmor := false
	for _, option := range securityOptions {
		switch {
		case option == "name=rootless" || strings.HasPrefix(option, "name=rootless,"):
			return false, nil
		case option == "name=apparmor" || strings.HasPrefix(option, "name=apparmor,"):
			apparmor = true
		default:
		}
	}

	return apparmor, nil
}

// The runtime matches a name filter loosely, so the exact name decides.
func (runtime dockerRuntime) exists(
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

func (runtime dockerRuntime) inspectContainers(
	ctx context.Context, outErr io.Writer, names ...string,
) ([]dockerInspection, error) {
	return inspectListed[dockerInspection](ctx, runtime, outErr, "container", names)
}

// inspectListed inspects objects that may have been removed since they were
// listed, for example by another deployment. Docker fails a bulk inspection
// when any object is gone, so each is then inspected alone and the gone ones
// are skipped.
func inspectListed[T any](
	ctx context.Context, runtime dockerRuntime, outErr io.Writer, kind string, ids []string,
) ([]T, error) {
	inspected, gone, err := inspectDocker[T](ctx, runtime, outErr, kind, ids...)
	if !gone || len(ids) == 1 {
		return inspected, err
	}
	inspected = nil
	for _, id := range ids {
		one, gone, err := inspectDocker[T](ctx, runtime, outErr, kind, id)
		if err != nil {
			return nil, err
		}
		if !gone {
			inspected = append(inspected, one...)
		}
	}

	return inspected, nil
}

// inspectDocker reports an object that does not exist apart from other
// failures, and writes only the latter's diagnostics.
func inspectDocker[T any](
	ctx context.Context, runtime dockerRuntime, outErr io.Writer, kind string, ids ...string,
) ([]T, bool, error) {
	var diagnostic bytes.Buffer
	output, err := runtime.output(
		ctx, nil, &diagnostic, append([]string{kind, dockerInspectCommand}, ids...)...,
	)
	if err != nil {
		if dockerObjectMissing(diagnostic.String()) {
			return nil, true, nil
		}
		if outErr != nil {
			_, _ = outErr.Write(diagnostic.Bytes())
		}

		return nil, false, err
	}
	var inspected []T
	if err := json.Unmarshal([]byte(output), &inspected); err != nil {
		return nil, false, fmt.Errorf("decode %s inspection: %w", kind, err)
	}

	return inspected, false, nil
}

// Docker reports a missing container as "No such container: <id>" and a
// missing network as "network <id> not found".
func dockerObjectMissing(diagnostic string) bool {
	return strings.Contains(diagnostic, "No such container") ||
		strings.Contains(diagnostic, "No such network") ||
		(strings.Contains(diagnostic, "network ") && strings.Contains(diagnostic, " not found"))
}

// Docker's own reply shapes. Nothing above this file sees them.
//
//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerInspection struct {
	ID              string                `json:"Id"`
	Name            string                `json:"Name"`
	State           dockerState           `json:"State"`
	Config          dockerConfig          `json:"Config"`
	HostConfig      dockerHostConfig      `json:"HostConfig"`
	NetworkSettings dockerNetworkSettings `json:"NetworkSettings"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerState struct {
	Status    string    `json:"Status"`
	ExitCode  int       `json:"ExitCode"`
	StartedAt time.Time `json:"StartedAt"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerConfig struct {
	Labels map[string]string `json:"Labels"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerHostConfig struct {
	NetworkMode string `json:"NetworkMode"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerNetworkSettings struct {
	Ports    map[string][]dockerPortBinding     `json:"Ports"`
	Networks map[string]dockerNetworkAttachment `json:"Networks"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerNetworkAttachment struct {
	Aliases []string `json:"Aliases"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

//nolint:tagliatelle // Docker inspect emits these exact field names.
type dockerNetwork struct {
	Name   string            `json:"Name"`
	Driver string            `json:"Driver"`
	Labels map[string]string `json:"Labels"`
}

func (inspection dockerInspection) listing() ContainerListing {
	ports := []ContainerPort{}
	for _, published := range slices.Sorted(maps.Keys(inspection.NetworkSettings.Ports)) {
		rawPort, protocol, _ := strings.Cut(published, "/")
		containerPort, err := strconv.Atoi(rawPort)
		if err != nil {
			continue
		}
		for _, binding := range inspection.NetworkSettings.Ports[published] {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil {
				continue
			}
			ports = append(ports, ContainerPort{
				HostIP: binding.HostIP, HostPort: hostPort,
				ContainerPort: containerPort, Protocol: protocol,
			})
		}
	}

	return ContainerListing{
		ID:        inspection.ID,
		Names:     []string{strings.TrimPrefix(inspection.Name, "/")},
		Labels:    inspection.Config.Labels,
		State:     inspection.State.Status,
		Lifecycle: dockerLifecycle(inspection.State.Status),
		ExitCode:  inspection.State.ExitCode,
		Ports:     ports,
		StartedAt: inspection.State.StartedAt,
	}
}

func dockerLifecycle(status string) ContainerLifecycle {
	switch status {
	case "running":
		return ContainerLifecycleRunning
	case "restarting":
		return ContainerLifecycleRestarting
	case "paused":
		return ContainerLifecyclePaused
	case "created", "exited", "dead", "removing":
		return ContainerLifecycleStopped
	default:
		return ContainerLifecycleUnknown
	}
}

// Docker names the network for a container on a user-defined bridge, so
// only the modes that never bridge are excluded.
func (inspection dockerInspection) attachment() ContainerAttachment {
	mode := inspection.HostConfig.NetworkMode
	networks := make(map[string]NetworkMembership, len(inspection.NetworkSettings.Networks))
	for name, membership := range inspection.NetworkSettings.Networks {
		networks[name] = NetworkMembership(membership)
	}

	return ContainerAttachment{
		Bridged:  mode != "host" && mode != "none" && !strings.HasPrefix(mode, "container:"),
		Networks: networks,
	}
}

func (network dockerNetwork) network() ContainerNetwork {
	bridge := network.Driver == dockerDefaultBridge

	return ContainerNetwork{
		Name:       network.Name,
		Bridge:     bridge,
		DNSEnabled: bridge && network.Name != dockerDefaultBridge,
		Labels:     network.Labels,
	}
}
