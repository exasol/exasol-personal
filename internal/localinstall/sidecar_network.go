// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localinstall

import (
	"context"
	"fmt"
	"maps"
	"os"
	goruntime "runtime"
	"slices"
	"strings"

	"github.com/exasol/exasol-personal/internal/sidecar"
)

const serviceNetworkUnsupportedLabel = "com.exasol.launcher.service-network-unsupported"

// ServiceNetworkUnsupportedError reports that Podman failed to set up the
// deployment network with an nftables error.
type ServiceNetworkUnsupportedError struct {
	WSL bool
}

func (*ServiceNetworkUnsupportedError) Error() string {
	return "Podman could not set up the deployment network (nftables error)"
}

func newServiceNetworkUnsupportedError() error {
	return &ServiceNetworkUnsupportedError{WSL: hostUsesWSL()}
}

// The Windows launcher runs Podman in a WSL machine.
func hostUsesWSL() bool {
	if goruntime.GOOS == "windows" {
		return true
	}
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")

	return err == nil && strings.Contains(strings.ToLower(string(release)), "microsoft")
}

func serviceNetworkUnsupported(err error) bool {
	stderr := CommandStderr(err)

	return strings.Contains(stderr, "netavark") && strings.Contains(stderr, "nftables error")
}

func withoutServiceNetwork(spec ContainerSpec) ContainerSpec {
	spec.Network = ""
	spec.NetworkAlias = ""
	labels := make(map[string]string, len(spec.Labels)+1)
	maps.Copy(labels, spec.Labels)
	labels[serviceNetworkUnsupportedLabel] = "true"
	spec.Labels = labels

	return spec
}

func lacksServiceNetwork(container ContainerAttachment) bool {
	return container.Labels[serviceNetworkUnsupportedLabel] != ""
}

func (runtime *SidecarRuntime) NetworkName() string {
	return SidecarNetworkName(runtime.owner)
}

// SidecarNetworkName is the network a deployment's services share, named for
// the deployment's container.
func SidecarNetworkName(owner string) string {
	return owner + "-services"
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
	if lacksServiceNetwork(container) {
		return false, newServiceNetworkUnsupportedError()
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
	if err != nil || !present || lacksServiceNetwork(container) {
		return false, err
	}
	if !container.Bridged {
		return true, nil
	}
	network, joined := container.Member(runtime.NetworkName())

	return joined && !slices.Contains(network.Aliases, sidecar.DatabaseAlias), nil
}

// ServiceNetworkSupported fails while the running database lacks the deployment
// network because the host cannot provide it.
func (runtime *SidecarRuntime) ServiceNetworkSupported(ctx context.Context) error {
	container, present, err := runtime.client.Attachment(ctx, runtime.owner, nil)
	if err != nil || !present {
		return err
	}
	if lacksServiceNetwork(container) {
		return newServiceNetworkUnsupportedError()
	}

	return nil
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
