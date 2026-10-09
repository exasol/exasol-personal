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

	return containers, nil
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

func (runtime *MacVMRuntime) OpenPorts(
	ctx context.Context,
	ports []sidecar.PublishedPort,
) error {
	forwards := macForwards{
		read: runtime.readSidecarForwards,
		// The runner is only needed once a mapping actually changes, so a
		// deployment with nothing to publish needs no runtime artifact.
		update: func(ctx context.Context, args ...string) error {
			runnerPath, err := runtime.resolveRunnerPath(ctx)
			if err != nil {
				return err
			}

			return runtime.runnerCommand(
				ctx, runnerPath, append([]string{"forward"}, args...), nil, nil,
			)
		},
	}

	return forwards.open(ctx, ports)
}

func (runtime *MacVMRuntime) OpenedPorts(context.Context) ([]sidecar.PublishedPort, error) {
	return macForwards{read: runtime.readSidecarForwards}.opened()
}

// A deployment whose VM has never started publishes nothing yet.
func (runtime *MacVMRuntime) readSidecarForwards() (map[string]runnerForwardState, error) {
	state, err := readRunnerState(runtime.paths.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]runnerForwardState{}, nil
	}
	if err != nil {
		return nil, err
	}

	return state.Forwards, nil
}

// macForwards publishes guest endpoints on the host through the runner's
// forward porcelain, which applies mappings while the VM keeps running.
type macForwards struct {
	read   func() (map[string]runnerForwardState, error)
	update func(ctx context.Context, args ...string) error
}

func (forwards macForwards) opened() ([]sidecar.PublishedPort, error) {
	current, err := forwards.read()
	if err != nil {
		return nil, err
	}
	var ports []sidecar.PublishedPort
	for _, name := range slices.Sorted(maps.Keys(current)) {
		service, index, owned := parseForwardName(name)
		if !owned {
			continue
		}
		forward := current[name]
		ports = append(ports, sidecar.PublishedPort{
			Sidecar: service, Index: index, HostIP: forward.HostIP,
			HostPort: forward.HostPort, RuntimePort: forward.GuestPort,
		})
	}

	return ports, nil
}

func (forwards macForwards) open(ctx context.Context, ports []sidecar.PublishedPort) error {
	current, err := forwards.read()
	if err != nil {
		return err
	}
	wanted := make(map[string]runnerForwardState, len(ports))
	for _, port := range ports {
		// A guest port appears once the container serves the endpoint, so an
		// unbound declaration withdraws its forward and keeps enablement.
		if port.RuntimePort == 0 {
			continue
		}
		wanted[forwardName(port)] = runnerForwardState{
			HostIP: port.HostIP, HostPort: port.HostPort, GuestPort: port.RuntimePort,
		}
	}
	// Forward ownership survives a removed container, so interrupted cleanup is retryable.
	for _, name := range slices.Sorted(maps.Keys(current)) {
		if _, _, owned := parseForwardName(name); !owned || current[name] == wanted[name] {
			continue
		}
		if err := forwards.update(ctx, "remove", name); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(wanted)) {
		port := wanted[name]
		if current[name] == port {
			continue
		}
		if err := forwards.update(ctx, "add", "--host-ip", port.HostIP,
			fmt.Sprintf("%s:%d:%d", name, port.GuestPort, port.HostPort)); err != nil {
			return err
		}
	}

	return nil
}

func forwardName(port sidecar.PublishedPort) string {
	return sidecarForwardPrefix + port.Sidecar + "." + strconv.Itoa(port.Index)
}

func parseForwardName(name string) (string, int, bool) {
	trimmed, found := strings.CutPrefix(name, sidecarForwardPrefix)
	if !found {
		return "", 0, false
	}
	service, suffix, found := strings.Cut(trimmed, ".")
	if !found || service == "" {
		return "", 0, false
	}
	index, err := strconv.Atoi(suffix)
	if err != nil {
		return "", 0, false
	}

	return service, index, true
}
