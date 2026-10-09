// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/sidecar"
	"github.com/exasol/exasol-personal/internal/task_runner"
	"github.com/exasol/exasol-personal/internal/tofu"
	"github.com/zclconf/go-cty/cty"
)

// tofuSidecarPortsVariable carries the launcher-managed ingress ports as a
// comma-separated list, which every infrastructure preset turns into firewall
// rules alongside its fixed database ports.
const tofuSidecarPortsVariable = "sidecar_ports"

func (backend *localBackend) OpenPorts(
	ctx context.Context,
	ports []sidecar.PublishedPort,
) error {
	return backend.runtime.OpenPorts(ctx, ports)
}

func (backend *localBackend) OpenedPorts(
	ctx context.Context,
) ([]sidecar.PublishedPort, error) {
	return backend.runtime.OpenedPorts(ctx)
}

// OpenPorts admits the declared sidecar ports through the provider firewall.
// Cloud sidecars bind the node's own address, so admission is what makes a
// declared endpoint reachable and it follows enablement rather than whether a
// container currently runs.
func (b *tofuBackend) OpenPorts(ctx context.Context, ports []sidecar.PublishedPort) error {
	if !b.hasTofu() {
		return nil
	}
	wanted := ingressPorts(ports)
	current, err := b.ingressPorts()
	if err != nil {
		return err
	}
	if slices.Equal(current, wanted) {
		return nil
	}
	if err := tofu.SetVariables(b.cfg, map[string]cty.Value{
		tofuSidecarPortsVariable: cty.StringVal(formatIngressPorts(wanted)),
	}); err != nil {
		return err
	}
	slog.Info("opening sidecar ports", "ports", wanted)
	logBuffer := task_runner.NewLogBuffer()
	if err := b.applyAction(ctx, "", logBuffer, logBuffer); err != nil {
		logBuffer.ReplayLogMessages(ctx)

		return fmt.Errorf("failed to open sidecar ports: %w", err)
	}

	return nil
}

// Cloud containers bind the node address directly, so the backend publishes no
// endpoint of its own.
func (*tofuBackend) OpenedPorts(context.Context) ([]sidecar.PublishedPort, error) {
	return nil, nil
}

func (b *tofuBackend) ingressPorts() ([]int, error) {
	values, err := b.loadCurrentTofuValues()
	if err != nil {
		return nil, err
	}
	value, configured := values[tofuSidecarPortsVariable]
	if !configured {
		return nil, nil
	}
	raw, err := ctyScalarToRawString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid value for %s: %w", tofuSidecarPortsVariable, err)
	}
	var ports []int
	for field := range strings.SplitSeq(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		port, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: %w", tofuSidecarPortsVariable, err)
		}
		ports = append(ports, port)
	}

	return ports, nil
}

func ingressPorts(ports []sidecar.PublishedPort) []int {
	numbers := make([]int, 0, len(ports))
	for _, port := range ports {
		if port.HostPort > 0 && !slices.Contains(numbers, port.HostPort) {
			numbers = append(numbers, port.HostPort)
		}
	}
	slices.Sort(numbers)

	return numbers
}

func formatIngressPorts(ports []int) string {
	fields := make([]string, 0, len(ports))
	for _, port := range ports {
		fields = append(fields, strconv.Itoa(port))
	}

	return strings.Join(fields, ",")
}

// sidecarIngressConfiguration reports the ingress value a reconfigured
// workspace keeps, so a configuration change leaves open ports open.
func (b *tofuBackend) sidecarIngressConfiguration() (string, error) {
	ports, err := b.ingressPorts()
	if err != nil {
		return "", err
	}

	return formatIngressPorts(ports), nil
}
