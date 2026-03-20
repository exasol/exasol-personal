// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"errors"
	"log/slog"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Constants from shared package
const (
	OpSkip = shared.OpSkip
)

var ErrNoResourcesPlanned = errors.New("no resources found to plan cleanup")

// BuildPlan constructs the static ordered cleanup plan.
func BuildPlan() CleanupPlan {
	return CleanupPlan{Phases: []Phase{
		{Name: "instances", Types: []ResourceType{ResourceComputeInstance}},
		{Name: "block-storage", Types: []ResourceType{ResourceBlockVolume}},
		{Name: "iam-api-keys", Types: []ResourceType{ResourceIAMAPIKey}},
		{Name: "iam-roles", Types: []ResourceType{ResourceIAMRole}},
		{Name: "networking", Types: []ResourceType{ResourcePrivateNetwork, ResourceSecurityGroup}},
		{Name: "ssh-keys", Types: []ResourceType{ResourceSSHKey}},
		{Name: "s3", Types: []ResourceType{ResourceSOSBucket}},
	}}
}

// PlanActions creates action list from resources & plan.
func PlanActions(details *DeploymentDetails, typeFilter []ResourceType) ([]Action, error) {
	if details == nil || len(details.Resources) == 0 {
		return nil, ErrNoResourcesPlanned
	}
	filter := map[ResourceType]struct{}{}
	for _, t := range typeFilter {
		filter[t] = struct{}{}
	}
	plan := BuildPlan()
	var actions []Action
	for _, phase := range plan.Phases {
		for _, resource := range details.Resources {
			if !containsType(phase.Types, resource.Ref.Type) {
				continue
			}
			if len(filter) > 0 {
				if _, ok := filter[resource.Ref.Type]; !ok {
					continue
				}
			}
			act := Action{Ref: resource.Ref, Op: opForResource(resource), Reason: ""}
			if resource.Protected {
				act.Op = OpSkip
				act.Reason = "protected"
			}
			actions = append(actions, act)
		}
	}
	if len(actions) == 0 {
		return nil, ErrNoResourcesPlanned
	}

	return actions, nil
}

func containsType(list []ResourceType, t ResourceType) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}

	return false
}

func opForResource(_ ResourceMeta) string {
	// For now everything is delete unless protected
	return "delete"
}

// ExecuteActions runs the actions. Executes deletion if execute=true.
//
//nolint:revive // 'execute' is an intentional flag to control dry-run vs execute behavior.
func ExecuteActions(ctx context.Context, zone string, actions []Action, execute bool) ([]Result, error) {
	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		res := Result{Action: action, Status: "planned"}
		if execute && action.Op != OpSkip {
			if err := deleteResource(ctx, zone, action.Ref); err != nil {
				res.Status = "failed"
				res.Error = err.Error()
				slog.Error(
					"cleanup failed",
					"id",
					action.Ref.ID,
					"type",
					action.Ref.Type,
					"error",
					err,
				)
			} else {
				res.Status = "success"
				slog.Info("cleanup success", "op", action.Op, "id",
					action.Ref.ID, "type", action.Ref.Type)
			}
		} else if action.Op == OpSkip {
			res.Status = "skipped"
		}
		results = append(results, res)
	}

	return results, nil
}
