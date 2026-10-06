// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

const (
	databaseAlias     = "database"
	bridgeNetworkMode = "bridge"
)

type serviceNetwork struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	DNSEnabled bool              `json:"dns_enabled"` //nolint:tagliatelle // Podman schema.
	Labels     map[string]string `json:"labels"`
}

type databaseNetworkInspection struct {
	HostConfig      databaseHostConfig      `json:"hostConfig"`
	NetworkSettings databaseNetworkSettings `json:"networkSettings"`
}

type databaseHostConfig struct {
	NetworkMode string `json:"networkMode"`
}

type databaseNetworkSettings struct {
	Networks map[string]networkAttachment `json:"networks"`
}

type networkAttachment struct {
	Aliases []string `json:"aliases"`
}

func (runtime *SidecarRuntime) NetworkName() string {
	return runtime.owner + "-services"
}

func (runtime *SidecarRuntime) EnsureNetwork(ctx context.Context) error {
	name := runtime.NetworkName()
	if _, err := runtime.command(
		ctx,
		"network",
		"create",
		"--ignore",
		"--driver",
		bridgeNetworkMode,
		labelFlag,
		sidecarOwnerLabel+"="+runtime.owner,
		name,
	); err != nil {
		return err
	}
	output, err := runtime.command(ctx, "network", "inspect", name)
	if err != nil {
		return err
	}
	var networks []serviceNetwork
	if err := json.Unmarshal(output, &networks); err != nil {
		return fmt.Errorf("decode deployment network: %w", err)
	}
	if len(networks) != 1 || networks[0].Name != name {
		return fmt.Errorf("runtime did not report deployment network %s", name)
	}

	return runtime.validateNetwork(networks[0])
}

// AttachDatabase reports whether explicit stop/start is needed to gain service DNS.
func (runtime *SidecarRuntime) AttachDatabase(ctx context.Context) (bool, error) {
	if runtime.DatabaseHost != "" {
		return false, nil
	}
	container, err := runtime.inspectDatabaseNetwork(ctx)
	if err != nil {
		return false, err
	}
	if container.HostConfig.NetworkMode != bridgeNetworkMode {
		return true, nil
	}
	if err := runtime.EnsureNetwork(ctx); err != nil {
		return false, err
	}
	if network, connected := container.NetworkSettings.Networks[runtime.NetworkName()]; connected {
		return !slices.Contains(network.Aliases, databaseAlias), nil
	}
	_, err = runtime.command(ctx, "network", "connect", "--alias", databaseAlias,
		runtime.NetworkName(), runtime.owner)

	return false, err
}

func (runtime *SidecarRuntime) DatabaseRestartRequired(ctx context.Context) (bool, error) {
	if runtime.DatabaseHost != "" {
		return false, nil
	}
	container, err := runtime.inspectDatabaseNetwork(ctx)
	if err != nil {
		return false, err
	}
	if container.HostConfig.NetworkMode != bridgeNetworkMode {
		return true, nil
	}
	network, connected := container.NetworkSettings.Networks[runtime.NetworkName()]

	return connected && !slices.Contains(network.Aliases, databaseAlias), nil
}

func (runtime *SidecarRuntime) RemoveNetwork(ctx context.Context) error {
	output, err := runtime.command(ctx, "network", "ls", "--format", "json")
	if err != nil {
		return err
	}
	var networks []serviceNetwork
	if err := json.Unmarshal(output, &networks); err != nil {
		return fmt.Errorf("decode deployment networks: %w", err)
	}
	for _, network := range networks {
		if network.Name != runtime.NetworkName() {
			continue
		}
		if err := runtime.validateNetwork(network); err != nil {
			return err
		}
		_, err := runtime.command(ctx, "network", "rm", network.Name)

		return err
	}

	return nil
}

func (runtime *SidecarRuntime) validateNetwork(network serviceNetwork) error {
	if network.Labels[sidecarOwnerLabel] != runtime.owner {
		return fmt.Errorf("network %s is not owned by this deployment", network.Name)
	}
	if network.Driver != bridgeNetworkMode || !network.DNSEnabled {
		return fmt.Errorf("deployment network %s requires a DNS-enabled bridge", network.Name)
	}

	return nil
}

func (runtime *SidecarRuntime) inspectDatabaseNetwork(
	ctx context.Context,
) (databaseNetworkInspection, error) {
	output, err := runtime.command(ctx, "container", "inspect", runtime.owner)
	if err != nil {
		return databaseNetworkInspection{}, err
	}
	var containers []databaseNetworkInspection
	if err := json.Unmarshal(output, &containers); err != nil {
		return databaseNetworkInspection{}, fmt.Errorf("decode database network settings: %w", err)
	}
	if len(containers) != 1 {
		return databaseNetworkInspection{}, fmt.Errorf(
			"runtime did not report database container %s",
			runtime.owner,
		)
	}

	return containers[0], nil
}
