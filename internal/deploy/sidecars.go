// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package deploy

import (
	"context"
	"errors"
	"maps"
	"runtime"
	"slices"

	"github.com/exasol/exasol-personal/assets/resources"
	"github.com/exasol/exasol-personal/internal/config"
	"github.com/exasol/exasol-personal/internal/sidecar"
)

type SidecarEndpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type SidecarResult struct {
	Name    string              `json:"name"`
	Enabled bool                `json:"enabled"`
	Hosts   []SidecarHostResult `json:"hosts"`
}

type SidecarHostResult struct {
	Name               string            `json:"name"`
	Running            bool              `json:"running"`
	State              string            `json:"state"`
	Reconciliation     string            `json:"reconciliation"`
	ExitCode           int               `json:"exitCode"`
	Error              string            `json:"error,omitempty"`
	LastOperationError string            `json:"lastOperationError,omitempty"`
	Endpoints          []SidecarEndpoint `json:"endpoints"`
}

type SidecarAvailability struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Architectures []string `json:"architectures"`
	Supported     bool     `json:"supported"`
	Enabled       bool     `json:"enabled"`
}

type sidecarOperations struct {
	deployment   config.DeploymentDir
	templates    sidecar.Resolver
	catalog      sidecar.Resolver
	architecture string
}

func ListSidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]SidecarAvailability, error) {
	var result []SidecarAvailability
	err := withDeploymentSharedLock(ctx, deployment, func(directory config.DeploymentDir) error {
		ops := newSidecarOperations(directory, nil)
		document, err := config.ReadSidecars(directory)
		if err != nil {
			return err
		}
		if err := ops.loadCatalog(); err != nil {
			return err
		}
		entries := ops.catalog.Entries()
		result = make([]SidecarAvailability, 0, len(entries))
		for _, name := range slices.Sorted(maps.Keys(entries)) {
			entry := entries[name]
			result = append(result, SidecarAvailability{
				Name:          name,
				Description:   entry.Description,
				Architectures: slices.Clone(entry.Architectures),
				Supported:     slices.Contains(entry.Architectures, ops.architecture),
				Enabled:       containsSidecar(document, name),
			})
		}

		return nil
	})

	return result, err
}

func EnableSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return enableSidecarTemplate(ctx, deployment, nil, name)
}

// enableSidecarTemplate resolves the selection against templates assembled for
// this command before the embedded catalog, so a command can enable a sidecar
// the catalog does not carry.
func enableSidecarTemplate(
	ctx context.Context, deployment config.DeploymentDir, templates sidecar.Resolver,
	name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		templates,
		func(ops *sidecarOperations) (SidecarResult, error) {
			return ops.enable(name)
		},
	)
}

func DisableSidecar(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		nil,
		func(ops *sidecarOperations) (SidecarResult, error) {
			return ops.disable(name)
		},
	)
}

func GetSidecarStatus(
	ctx context.Context, deployment config.DeploymentDir, name string,
) (SidecarResult, error) {
	return withSidecar(
		ctx,
		deployment,
		nil,
		func(ops *sidecarOperations) (SidecarResult, error) {
			return ops.status(name)
		},
	)
}

func withSidecar(
	ctx context.Context, deployment config.DeploymentDir, templates sidecar.Resolver,
	action func(*sidecarOperations) (SidecarResult, error),
) (SidecarResult, error) {
	var result SidecarResult
	err := withDeploymentExclusiveLock(ctx, deployment, func(dir config.DeploymentDir) error {
		var err error
		result, err = action(newSidecarOperations(dir, templates))

		return err
	})

	return result, err
}

func newSidecarOperations(
	deployment config.DeploymentDir,
	templates sidecar.Resolver,
) *sidecarOperations {
	architecture := localLinuxAMD64
	if isLocalDeployment(deployment) {
		architecture = runtime.GOARCH
	}

	return &sidecarOperations{
		deployment: deployment, templates: templates, architecture: architecture,
	}
}

func (ops *sidecarOperations) loadCatalog() error {
	if ops.catalog != nil {
		return nil
	}
	embedded, err := sidecar.LoadCatalog(resources.SidecarCatalogYAML)
	if err != nil {
		return err
	}
	catalogs := sidecar.Catalogs{embedded}
	if ops.templates != nil {
		catalogs = sidecar.Catalogs{ops.templates, embedded}
	}
	ops.catalog = catalogs

	return nil
}

func (ops *sidecarOperations) enable(name string) (SidecarResult, error) {
	if err := ops.mutationAllowed(); err != nil {
		return SidecarResult{}, err
	}
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	if !containsSidecar(document, name) {
		if err := ops.loadCatalog(); err != nil {
			return SidecarResult{}, err
		}
		if _, err := ops.catalog.Resolve(name, ops.architecture); err != nil {
			return SidecarResult{}, err
		}
	}
	if _, err := materializeSidecarLocked(
		ops.deployment, document, ops.catalog, name, ops.architecture,
	); err != nil {
		return SidecarResult{}, err
	}

	return ops.status(name)
}

func (ops *sidecarOperations) disable(name string) (SidecarResult, error) {
	if err := ops.mutationAllowed(); err != nil {
		return SidecarResult{}, err
	}
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return SidecarResult{}, err
	}
	document.Containers = slices.DeleteFunc(
		document.Containers,
		func(container sidecar.Container) bool { return container.Name == name },
	)
	if err := config.WriteSidecars(ops.deployment, document); err != nil {
		return SidecarResult{}, err
	}

	return ops.status(name)
}

func (ops *sidecarOperations) status(name string) (SidecarResult, error) {
	document, err := config.ReadSidecars(ops.deployment)
	if err != nil {
		return SidecarResult{}, err
	}

	return SidecarResult{
		Name:    name,
		Enabled: containsSidecar(document, name),
		Hosts:   []SidecarHostResult{},
	}, nil
}

func (ops *sidecarOperations) mutationAllowed() error {
	state, err := config.ReadExasolPersonalState(ops.deployment)
	if err != nil {
		return err
	}
	workflow, err := state.GetWorkflowState()
	if err != nil {
		return err
	}
	switch workflow.(type) {
	case *config.WorkflowStateInitialized,
		*config.WorkflowStateStopped,
		*config.WorkflowStateRunning:
		return nil
	default:
		return errors.New(
			"sidecar changes require an initialized, running, or stopped deployment",
		)
	}
}

func containsSidecar(document sidecar.Document, name string) bool {
	return slices.ContainsFunc(
		document.Containers,
		func(container sidecar.Container) bool { return container.Name == name },
	)
}
