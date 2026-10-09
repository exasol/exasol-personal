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

type SidecarResult struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type SidecarAvailability struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Architectures []string `json:"architectures"`
	Supported     bool     `json:"supported"`
	Enabled       bool     `json:"enabled"`
}

type sidecarOperations struct {
	deployment config.DeploymentDir
	templates  sidecar.Resolver
}

func ListSidecars(
	ctx context.Context,
	deployment config.DeploymentDir,
) ([]SidecarAvailability, error) {
	// The catalog stays browsable before a deployment exists.
	if _, err := config.ReadInfrastructureManifest(deployment); err != nil {
		return catalogAvailability(sidecar.Document{}, runtime.GOARCH)
	}
	var result []SidecarAvailability
	err := withDeploymentSharedLock(ctx, deployment, func(directory config.DeploymentDir) error {
		document, err := config.ReadSidecars(directory)
		if err != nil {
			return err
		}
		result, err = catalogAvailability(document, runtime.GOARCH)

		return err
	})

	return result, err
}

func catalogAvailability(
	document sidecar.Document,
	architecture string,
) ([]SidecarAvailability, error) {
	catalog, err := sidecarCatalog(nil)
	if err != nil {
		return nil, err
	}
	entries := catalog.Entries()
	result := make([]SidecarAvailability, 0, len(entries))
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		entry := entries[name]
		result = append(result, SidecarAvailability{
			Name:          name,
			Description:   entry.Description,
			Architectures: slices.Clone(entry.Architectures),
			Supported:     slices.Contains(entry.Architectures, architecture),
			Enabled:       containsSidecar(document, name),
		})
	}

	return result, nil
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
		result, err = action(&sidecarOperations{deployment: dir, templates: templates})

		return err
	})

	return result, err
}

// sidecarCatalog resolves selections against templates assembled for this
// command before the launcher's embedded entries.
func sidecarCatalog(templates sidecar.Resolver) (sidecar.Resolver, error) {
	embedded, err := sidecar.LoadCatalog(resources.SidecarCatalogYAML)
	if err != nil {
		return nil, err
	}
	if templates == nil {
		return sidecar.Catalogs{embedded}, nil
	}

	return sidecar.Catalogs{templates, embedded}, nil
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
		catalog, err := sidecarCatalog(ops.templates)
		if err != nil {
			return SidecarResult{}, err
		}
		container, err := catalog.Resolve(name, runtime.GOARCH)
		if err != nil {
			return SidecarResult{}, err
		}
		document.Containers = append(document.Containers, container)
		if err := config.WriteSidecars(ops.deployment, document); err != nil {
			return SidecarResult{}, err
		}
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

	return SidecarResult{Name: name, Enabled: containsSidecar(document, name)}, nil
}

// mutationAllowed refuses the states where a deployment is mid-operation and
// its saved intent could not be acted on coherently.
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
