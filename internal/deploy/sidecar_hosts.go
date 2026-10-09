// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"io"
	"runtime"
	"time"

	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/localinstall"
	"github.com/exasol/exasol-personal/internal/localruntime"
	"github.com/exasol/exasol-personal/internal/remote"
)

type sidecarHost struct {
	name         string
	architecture string
	provider     localinstall.SidecarProvider
	running      func(context.Context) (bool, error)
	databasePort string
}

const (
	sidecarSSHMaxBackoffSeconds = 5
	// Cloud nodes run their database under this service, so sidecars that
	// belong to it follow the node's own start, stop, and restart handling.
	cloudDeploymentService = "c4_cloud_command.service"
)

func localSidecarHost(selected localruntime.Runtime) sidecarHost {
	return sidecarHost{
		name: "local", architecture: runtime.GOARCH, provider: selected,
		running: func(ctx context.Context) (bool, error) {
			status, err := selected.Status(ctx)
			if err != nil {
				return false, err
			}

			return status.Running, nil
		},
	}
}

func (backend *localBackend) SidecarHosts(context.Context) ([]sidecarHost, error) {
	host := localSidecarHost(backend.runtime)
	host.architecture = backend.goarch

	return []sidecarHost{host}, nil
}

func (backend *tofuBackend) SidecarHosts(context.Context) ([]sidecarHost, error) {
	state, err := config.ReadExasolPersonalState(backend.deployment)
	if err != nil {
		return nil, err
	}
	workflow, err := state.GetWorkflowState()
	if err != nil {
		return nil, err
	}
	if _, initialized := workflow.(*config.WorkflowStateInitialized); initialized {
		return nil, nil
	}
	info, err := config.ReadDeploymentInfo(backend.deployment)
	if err != nil {
		return nil, err
	}
	if len(info.Nodes) == 0 {
		return nil, errors.New("cloud deployment has no nodes")
	}
	hosts := make([]sidecarHost, 0, len(info.Nodes))
	for _, name := range info.ListNodes() {
		node := info.Nodes[name]
		provider := &cloudSidecarProvider{
			deployment: backend.deployment,
			node:       name,
			address:    node.PrivateIp,
		}
		hosts = append(hosts, sidecarHost{
			name: name, architecture: localLinuxAMD64, provider: provider,
			running: provider.SidecarHostRunning, databasePort: node.Database.DbPort,
		})
	}

	return hosts, nil
}

type cloudSidecarProvider struct {
	deployment config.DeploymentDir
	node       string
	address    string
}

func (provider *cloudSidecarProvider) SidecarHostRunning(context.Context) (bool, error) {
	state, err := config.ReadExasolPersonalState(provider.deployment)
	if err != nil {
		return false, err
	}
	workflow, err := state.GetWorkflowState()
	if err != nil {
		return false, err
	}
	switch workflow.(type) {
	case *config.WorkflowStateInitialized, *config.WorkflowStateStopped:
		return false, nil
	default:
		return true, nil
	}
}

func (provider *cloudSidecarProvider) Sidecars(
	context.Context,
) (localinstall.SidecarManager, error) {
	if provider.address == "" {
		return nil, errors.New("cloud sidecar host has no private database address")
	}
	connection, err := sshRemoteForNodeUnsafe(provider.deployment, provider.node)
	if err != nil {
		return nil, err
	}
	owner, err := localinstall.ContainerName(provider.deployment)
	if err != nil {
		return nil, err
	}
	manager, err := localinstall.NewSystemdSidecarRuntime(
		&sidecarSSHEnvironment{connection}, owner, cloudDeploymentService,
	)
	if err != nil {
		return nil, err
	}
	manager.DatabaseHost = provider.address

	return manager, nil
}

type sidecarSSHEnvironment struct {
	connection interface {
		RunCommand(ctx context.Context, args []string, input io.Reader, out, errOut io.Writer) error
	}
}

func (environment *sidecarSSHEnvironment) Run(
	ctx context.Context,
	env map[string]string,
	input io.Reader,
	out, errOut io.Writer,
	command ...string,
) error {
	command, input, err := localinstall.CommandEnvironment(env, input, command)
	if err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var commandErr error
	err = PollWithBackoff(waitCtx, func(context.Context) (bool, error) {
		// Image pulls use the caller's deadline, independently of connection retries.
		commandErr = environment.connection.RunCommand(ctx, command, input, out, errOut)
		// Connection failures happen before stdin is consumed or the command executes.
		return !errors.Is(commandErr, remote.ErrFailedToConnect), commandErr
	}, WaitParams{
		InitialBackoff: 1, MaxBackoff: sidecarSSHMaxBackoffSeconds,
		LogPrefix: "waiting for sidecar host SSH",
	})

	return errors.Join(err, commandErr)
}
