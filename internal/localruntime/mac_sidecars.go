// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package localruntime

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

const sidecarForwardPrefix = "sidecar."

type macSidecars struct {
	localinstall.SidecarManager

	readForwards func() (map[string]runnerForwardState, error)
	forward      func(context.Context, ...string) error
}

func (runtime *MacVMRuntime) Sidecars(ctx context.Context) (localinstall.SidecarManager, error) {
	owner, err := localinstall.ContainerName(runtime.deployment)
	if err != nil {
		return nil, err
	}
	runnerPath, err := runtime.resolveRunnerPath(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := localinstall.NewSidecarRuntime(
		newRunnerExecutionEnvironment(runnerPath, runtime.paths.WorkDir), owner,
	)
	if err != nil {
		return nil, err
	}
	containers.PublishPort = func(port sidecar.Port) string {
		// The runner reaches the guest through its VM address, not guest loopback.
		return "0.0.0.0::" + strconv.Itoa(port.ContainerPort) + "/tcp"
	}

	return &macSidecars{
		SidecarManager: containers,
		readForwards: func() (map[string]runnerForwardState, error) {
			state, err := readRunnerState(runtime.paths.StatePath)
			if err != nil {
				return nil, err
			}

			return state.Forwards, nil
		},
		forward: func(ctx context.Context, args ...string) error {
			return runtime.runnerCommand(
				ctx,
				runnerPath,
				append([]string{"forward"}, args...),
				nil,
				nil,
			)
		},
	}, nil
}

func (runtime *MacVMRuntime) SidecarHostRunning(ctx context.Context) (bool, error) {
	if _, err := os.Stat(runtime.paths.VMDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}
	status, err := runnerCommandJSON[RuntimeStatus](ctx, runtime, "status")
	if err != nil {
		return false, err
	}

	return status.Running, nil
}

func (manager *macSidecars) List(ctx context.Context) ([]localinstall.SidecarContainer, error) {
	containers, err := manager.SidecarManager.List(ctx)
	if err != nil {
		return nil, err
	}
	forwards, err := manager.readForwards()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for index := range containers {
		container := &containers[index]
		seen[container.Name()] = true
		ports := []localinstall.SidecarPort{}
		for _, name := range slices.Sorted(maps.Keys(forwards)) {
			if forwardSidecarName(name) != container.Name() {
				continue
			}
			forward := forwards[name]
			port := localinstall.SidecarPort{
				HostIP: forward.HostIP, HostPort: forward.HostPort, Protocol: "tcp",
			}
			for _, guest := range container.Ports {
				if guest.HostPort == forward.GuestPort {
					port.ContainerPort, port.Protocol = guest.ContainerPort, guest.Protocol

					break
				}
			}
			// Unmatched forwards also prevent publication from matching desired state.
			ports = append(ports, port)
		}
		container.Ports = ports
	}
	// Forward ownership survives a removed container so interrupted cleanup is retryable.
	for _, name := range slices.Sorted(maps.Keys(forwards)) {
		service := forwardSidecarName(name)
		if service == "" || seen[service] {
			continue
		}
		seen[service] = true
		containers = append(containers, localinstall.SidecarContainer{
			State: "stopped", Labels: map[string]string{localinstall.SidecarNameLabel: service},
		})
	}

	return containers, nil
}

func (manager *macSidecars) Ensure(
	ctx context.Context, definition sidecar.Container, sources sidecar.Sources,
) error {
	if err := manager.SidecarManager.Ensure(ctx, definition, sources); err != nil {
		return err
	}
	containers, err := manager.SidecarManager.List(ctx)
	if err != nil {
		return err
	}
	var ports []localinstall.SidecarPort
	for _, container := range containers {
		if container.Name() == definition.Name {
			ports = container.Ports
		}
	}
	wanted := map[string]runnerForwardState{}
	for index, port := range definition.Defaults().Ports {
		if port.HostPort == 0 {
			continue
		}
		guestPort := 0
		for _, guest := range ports {
			if guest.ContainerPort == port.ContainerPort && guest.HostPort > 0 {
				guestPort = guest.HostPort
				break
			}
		}
		if guestPort == 0 {
			return fmt.Errorf(
				"sidecar %s: no guest publication for port %d",
				definition.Name,
				port.ContainerPort,
			)
		}
		name := sidecarForwardPrefix + definition.Name + "." + strconv.Itoa(index)
		wanted[name] = runnerForwardState{
			HostIP: port.HostIP, HostPort: port.HostPort, GuestPort: guestPort,
		}
	}
	current, err := manager.readForwards()
	if err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(current)) {
		if forwardSidecarName(name) == definition.Name && current[name] != wanted[name] {
			if err := manager.forward(ctx, "remove", name); err != nil {
				return err
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wanted)) {
		port := wanted[name]
		if current[name] == port {
			continue
		}
		if err := manager.forward(ctx, "add", "--host-ip", port.HostIP,
			fmt.Sprintf("%s:%d:%d", name, port.GuestPort, port.HostPort)); err != nil {
			return err
		}
	}

	return nil
}

func (manager *macSidecars) Remove(ctx context.Context, service string) error {
	forwards, err := manager.readForwards()
	if err != nil {
		return err
	}
	var failures []error
	for _, name := range slices.Sorted(maps.Keys(forwards)) {
		if forwardSidecarName(name) == service {
			failures = append(failures, manager.forward(ctx, "remove", name))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}

	return manager.SidecarManager.Remove(ctx, service)
}

func forwardSidecarName(name string) string {
	if !strings.HasPrefix(name, sidecarForwardPrefix) {
		return ""
	}
	service, _, found := strings.Cut(strings.TrimPrefix(name, sidecarForwardPrefix), ".")
	if !found {
		return ""
	}

	return service
}
