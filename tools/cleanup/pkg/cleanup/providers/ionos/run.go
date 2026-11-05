// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	ionoscloud "github.com/ionos-cloud/sdk-go/v6"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

// Constants from shared package.
const (
	OpSkip   = shared.OpSkip
	OpDelete = "delete"
)

var ErrNoResourcesPlanned = errors.New("no resources found to plan cleanup")

const reasonCascade = "removed with datacenter"

// deletionOrder lists resource types so that nothing is deleted while a
// resource that depends on it still exists. IP blocks stay reserved until the
// servers using them are gone, so they come last.
var deletionOrder = []ResourceType{
	ResourceDatacenter,
	ResourceServer,
	ResourceVolume,
	ResourceLAN,
	ResourceIPBlock,
}

// PlanActions builds the cleanup plan for a deployment. When the plan deletes
// the deployment's own datacenter, that removes the servers, volumes and LANs
// inside it, so those are listed as skipped; resources in any other datacenter
// are deleted one by one and that datacenter is left alone. IP blocks live
// outside datacenters and are always deleted.
func PlanActions(details *DeploymentDetails, typeFilter []ResourceType) ([]Action, error) {
	if details == nil || len(details.Resources) == 0 {
		return nil, ErrNoResourcesPlanned
	}

	planned := orderedResources(details.Resources, typeFilter)
	deletedDatacenters := datacenterIDs(planned)
	var skipped, deletions []Action
	for _, res := range planned {
		if _, cascades := deletedDatacenters[res.Ref.ParentID]; cascades {
			skipped = append(skipped, Action{Ref: res.Ref, Op: OpSkip, Reason: reasonCascade})
		} else {
			deletions = append(deletions, Action{Ref: res.Ref, Op: OpDelete})
		}
	}

	actions := append(skipped, deletions...)
	if len(actions) == 0 {
		return nil, ErrNoResourcesPlanned
	}

	return actions, nil
}

func datacenterIDs(resources []ResourceMeta) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, res := range resources {
		if res.Ref.Type == ResourceDatacenter {
			ids[res.Ref.ID] = struct{}{}
		}
	}

	return ids
}

// orderedResources returns the resources allowed by typeFilter (all when empty)
// in deletionOrder.
func orderedResources(resources []ResourceMeta, typeFilter []ResourceType) []ResourceMeta {
	var ordered []ResourceMeta
	for _, resourceType := range deletionOrder {
		if len(typeFilter) > 0 && !slices.Contains(typeFilter, resourceType) {
			continue
		}
		for _, res := range resources {
			if res.Ref.Type == resourceType {
				ordered = append(ordered, res)
			}
		}
	}

	return ordered
}

// ExecuteActions runs the planned actions in order, deleting only when execute
// is true. A failed deletion is recorded and the remaining actions still run.
//
//nolint:revive // 'execute' is an intentional flag to control dry-run vs execute behavior.
func ExecuteActions(ctx context.Context, actions []Action, execute bool) ([]Result, error) {
	var api *ionoscloud.APIClient
	if execute {
		var err error
		if api, err = newClient(); err != nil {
			return nil, err
		}
	}

	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		res := Result{Action: action, Status: shared.ResultStatusPlanned}
		switch {
		case action.Op == OpSkip:
			res.Status = shared.ResultStatusSkipped
		case execute:
			if err := deleteResource(ctx, api, action.Ref); err != nil {
				res.Status = shared.ResultStatusFailed
				res.Error = err.Error()
				slog.Error("cleanup failed",
					"id", action.Ref.ID, "type", action.Ref.Type, "error", err)
			} else {
				res.Status = shared.ResultStatusSuccess
				slog.Info("cleanup success",
					"op", action.Op, "id", action.Ref.ID, "type", action.Ref.Type)
			}
		}
		results = append(results, res)
	}

	return results, nil
}
