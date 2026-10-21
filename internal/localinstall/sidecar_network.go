// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"fmt"
	"slices"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

func (runtime *SidecarRuntime) NetworkName() string {
	return runtime.owner + "-services"
}

func (runtime *SidecarRuntime) EnsureNetwork(ctx context.Context) error {
	name := runtime.NetworkName()
	spec := NetworkSpec{
		Name:   name,
		DNS:    true,
		Labels: map[string]string{sidecarOwnerLabel: runtime.owner},
	}
	if err := runtime.client.CreateNetwork(ctx, spec, nil, nil); err != nil {
		return err
	}
	network, present, err := runtime.client.Network(ctx, name, nil)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("runtime did not report deployment network %s", name)
	}

	return runtime.validateNetwork(network)
}

// AttachDatabase reports whether explicit stop/start is needed to gain service DNS.
func (runtime *SidecarRuntime) AttachDatabase(ctx context.Context) (bool, error) {
	container, present, err := runtime.client.Attachment(ctx, runtime.owner, nil)
	if err != nil || !present {
		return false, err
	}
	if !container.Bridged {
		return true, nil
	}
	if err := runtime.EnsureNetwork(ctx); err != nil {
		return false, err
	}
	if network, joined := container.Member(runtime.NetworkName()); joined {
		return !slices.Contains(network.Aliases, sidecar.DatabaseAlias), nil
	}
	err = runtime.client.ConnectNetwork(
		ctx, runtime.NetworkName(), runtime.owner, []string{sidecar.DatabaseAlias}, nil, nil,
	)

	return false, err
}

func (runtime *SidecarRuntime) DatabaseRestartRequired(ctx context.Context) (bool, error) {
	container, present, err := runtime.client.Attachment(ctx, runtime.owner, nil)
	if err != nil || !present {
		return false, err
	}
	if !container.Bridged {
		return true, nil
	}
	network, joined := container.Member(runtime.NetworkName())

	return joined && !slices.Contains(network.Aliases, sidecar.DatabaseAlias), nil
}

func (runtime *SidecarRuntime) RemoveNetwork(ctx context.Context) error {
	networks, err := runtime.client.Networks(ctx, nil)
	if err != nil {
		return err
	}
	for _, network := range networks {
		if network.Name != runtime.NetworkName() {
			continue
		}
		if err := runtime.validateNetwork(network); err != nil {
			return err
		}

		return runtime.client.RemoveNetwork(ctx, network.Name, nil, nil)
	}

	return nil
}

func (runtime *SidecarRuntime) validateNetwork(network ContainerNetwork) error {
	if network.Labels[sidecarOwnerLabel] != runtime.owner {
		return fmt.Errorf("network %s is not owned by this deployment", network.Name)
	}
	if !network.Bridge || !network.DNSEnabled {
		return fmt.Errorf("deployment network %s requires a DNS-enabled bridge", network.Name)
	}

	return nil
}
