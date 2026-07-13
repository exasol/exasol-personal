// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import (
	"context"
	"errors"
	"log/slog"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

// Constants from shared package.
const (
	OpSkip   = shared.OpSkip
	OpDelete = "delete"
)

var ErrNoResourcesPlanned = errors.New("no resources found to plan cleanup")

// reason recorded for contained resources that are removed as part of the
// cascading resource-group deletion rather than deleted individually.
const reasonCascade = "removed with resource group"

// PlanActions builds the cleanup plan for a deployment. Azure cleanup is
// resource-group scoped: contained resources are listed as informational skip
// actions (they are removed by the cascading delete) and the resource group is
// the single executed deletion.
//
// A type filter narrows which contained resources are shown; the resource-group
// deletion is included unless the filter explicitly omits the resource-group
// type.
func PlanActions(details *DeploymentDetails, typeFilter []ResourceType) ([]Action, error) {
	if details == nil || len(details.Resources) == 0 {
		return nil, ErrNoResourcesPlanned
	}

	filter := map[ResourceType]struct{}{}
	for _, t := range typeFilter {
		filter[t] = struct{}{}
	}
	included := func(t ResourceType) bool {
		if len(filter) == 0 {
			return true
		}
		_, ok := filter[t]

		return ok
	}

	var (
		actions   []Action
		rgActions []Action
	)
	for _, resource := range details.Resources {
		if !included(resource.Ref.Type) {
			continue
		}
		if resource.Ref.Type == ResourceResourceGroup {
			op := OpDelete
			reason := ""
			if resource.Protected {
				op = OpSkip
				reason = "protected"
			}
			rgActions = append(rgActions, Action{Ref: resource.Ref, Op: op, Reason: reason})

			continue
		}
		// Contained resources are informational: the resource-group deletion
		// removes them. They are never deleted individually.
		actions = append(actions, Action{Ref: resource.Ref, Op: OpSkip, Reason: reasonCascade})
	}

	// The resource-group deletion is ordered last so the plan reads as
	// "these resources are covered, and this is the operation that removes them".
	actions = append(actions, rgActions...)

	if len(actions) == 0 {
		return nil, ErrNoResourcesPlanned
	}

	return actions, nil
}

// ExecuteActions runs the planned actions. Only the resource-group deletion
// performs work, and only when execute is true.
//
//nolint:revive // 'execute' is an intentional flag to control dry-run vs execute behavior.
func ExecuteActions(
	ctx context.Context,
	subscriptionID string,
	actions []Action,
	execute bool,
) ([]Result, error) {
	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		res := Result{Action: action, Status: "planned"}
		switch {
		case action.Op == OpSkip:
			res.Status = "skipped"
		case execute:
			if err := deleteResourceGroup(ctx, subscriptionID, action.Ref.ID); err != nil {
				res.Status = "failed"
				res.Error = err.Error()
				slog.Error("cleanup failed",
					"id", action.Ref.ID, "type", action.Ref.Type, "error", err)
			} else {
				res.Status = "success"
				slog.Info("cleanup success",
					"op", action.Op, "id", action.Ref.ID, "type", action.Ref.Type)
			}
		}
		results = append(results, res)
	}

	return results, nil
}
