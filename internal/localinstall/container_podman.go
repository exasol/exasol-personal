// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// podmanBridgeDriver routes between containers on one host.
const podmanBridgeDriver = "bridge"

// Podman's spellings for the policies the launcher asks for. A second runtime
// brings its own table rather than new call sites.
//
//nolint:gochecknoglobals // Lookup tables, not state.
var (
	podmanPullPolicies = map[PullPolicy]string{
		PullAlways:       "always",
		PullIfNotPresent: "missing",
		PullNever:        "never",
	}
	podmanRestartPolicies = map[RestartPolicy]string{
		RestartNever:     "no",
		RestartOnFailure: "on-failure",
		RestartAlways:    "always",
	}
)

// podmanCreateArgs encodes a spec as `podman run` arguments. It is the only
// place that knows how this runtime spells any of it.
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
	if policy, named := podmanPullPolicies[spec.Pull]; named {
		args = append(args, "--pull", policy)
	}
	if policy, named := podmanRestartPolicies[spec.Restart]; named {
		args = append(args, "--restart", policy)
	}

	return args
}

func podmanResourceArgs(args []string, spec ContainerSpec) []string {
	if spec.SharedMemorySize != "" {
		args = append(args, "--shm-size="+spec.SharedMemorySize)
	}
	if spec.ProcessLimit != "" {
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

func podmanCreateNetworkArgs(spec NetworkSpec) []string {
	args := []string{"network", "create", "--ignore", "--driver", podmanBridgeDriver}
	if !spec.DNS {
		args = append(args, "--disable-dns")
	}
	for _, key := range slices.Sorted(maps.Keys(spec.Labels)) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}

	return append(args, spec.Name)
}

// decodeNetworks reads the runtime's own network listing.
func decodeNetworks(output string) ([]ContainerNetwork, error) {
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	var networks []ContainerNetwork
	if err := json.Unmarshal([]byte(output), &networks); err != nil {
		return nil, fmt.Errorf("decode container networks: %w", err)
	}

	return networks, nil
}

// podmanDiagnosticCommands reports what to run when a container fails, so the
// set stays with the runtime that understands it.
func podmanDiagnosticCommands(container string) [][]string {
	return [][]string{
		{"info"},
		{"ps", "-a"},
		{"container", "inspect", container},
		{"logs", container},
	}
}
