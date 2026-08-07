// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"errors"
	"log/slog"

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
)

// Constants from shared package
const (
	OpSkip = shared.OpSkip
)

var ErrNoResourcesPlanned = errors.New("no resources found to plan cleanup")

// BuildPlan constructs the static ordered cleanup plan.
func BuildPlan() CleanupPlan {
	return CleanupPlan{Phases: []Phase{
		{Name: "servers", Types: []ResourceType{ResourceServer}},
		{Name: "volumes", Types: []ResourceType{ResourceVolume}},
		{Name: "public-ips", Types: []ResourceType{ResourcePublicIP}},
		{Name: "network-interfaces", Types: []ResourceType{ResourceNetworkInterface}},
		{Name: "networks", Types: []ResourceType{ResourceNetwork}},
		{Name: "security-groups", Types: []ResourceType{ResourceSecurityGroup}},
		{Name: "objectstorage-buckets", Types: []ResourceType{ResourceObjectStorageBucket}},
		{Name: "objectstorage-credentials", Types: []ResourceType{ResourceObjectStorageCredential}},
		{Name: "objectstorage-credentials-groups", Types: []ResourceType{ResourceObjectStorageCredentialsGroup}},
	}}
}

// PlanActions creates action list from resources & plan.
func PlanActions(details *DeploymentDetails, typeFilter []ResourceType) ([]Action, error) {
	if details == nil || len(details.Resources) == 0 {
		return nil, ErrNoResourcesPlanned
	}
	filter := buildTypeFilter(typeFilter)
	plan := BuildPlan()
	var actions []Action
	for _, phase := range plan.Phases {
		actions = append(actions, actionsForPhase(phase, details.Resources, filter)...)
	}
	if len(actions) == 0 {
		return nil, ErrNoResourcesPlanned
	}

	return actions, nil
}

func buildTypeFilter(typeFilter []ResourceType) map[ResourceType]struct{} {
	filter := map[ResourceType]struct{}{}
	for _, t := range typeFilter {
		filter[t] = struct{}{}
	}

	return filter
}

// actionsForPhase builds the actions for resources belonging to a single phase.
func actionsForPhase(phase Phase, resources []ResourceMeta, filter map[ResourceType]struct{}) []Action {
	var actions []Action
	for _, resource := range resources {
		if !resourceMatchesPhase(resource, phase, filter) {
			continue
		}
		actions = append(actions, buildAction(resource))
	}

	return actions
}

func resourceMatchesPhase(resource ResourceMeta, phase Phase, filter map[ResourceType]struct{}) bool {
	if !containsType(phase.Types, resource.Ref.Type) {
		return false
	}
	if len(filter) > 0 {
		if _, ok := filter[resource.Ref.Type]; !ok {
			return false
		}
	}

	return true
}

func buildAction(resource ResourceMeta) Action {
	act := Action{Ref: resource.Ref, Op: opForResource(resource), Reason: ""}
	if resource.Protected {
		act.Op = OpSkip
		act.Reason = "protected"
	}

	return act
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
func ExecuteActions(ctx context.Context, projectId, region string, actions []Action, execute bool) ([]Result, error) {
	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		res := Result{Action: action, Status: "planned"}
		if execute && action.Op != OpSkip {
			if err := deleteResource(ctx, projectId, region, action.Ref); err != nil {
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
