// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package hetzner

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Constants from shared package
const (
	StateActive       = shared.StateActive
	StateProvisioning = shared.StateProvisioning
	StateStopped      = shared.StateStopped
	StateTerminated   = shared.StateTerminated
	StateUnknown      = shared.StateUnknown
)

// deploymentIDLabel is the label key used to identify deployments
const deploymentIDLabel = "deployment_id"

// CollectDeploymentDetails enumerates resources for a single deployment in Hetzner Cloud
func CollectDeploymentDetails(
	ctx context.Context,
	deploymentID string,
) (*DeploymentDetails, error) {
	client := newHCloudClient()

	details := &DeploymentDetails{
		Summary: DeploymentSummary{
			ID:        deploymentID,
			Provider:  ProviderName,
			Region:    "",
			Owner:     "",
			CreatedAt: time.Time{},
			State:     StateUnknown,
		},
	}

	var earliest *time.Time
	hasServers := false
	hasActive := false
	hasStopped := false

	// Discover servers by label
	servers, err := client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: fmt.Sprintf("%s=%s", deploymentIDLabel, deploymentID),
		},
	})
	if err != nil {
		slog.Debug("list servers failed", "error", err)
	} else {
		for _, server := range servers {
			hasServers = true
			state := serverStateToSimple(server.Status)

			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    serverARN(server.ID),
					Type:   ResourceServer,
					Region: server.Datacenter.Location.Name,
					ID:     strconv.FormatInt(server.ID, 10),
				},
				Tags: server.Labels,
				Attr: map[string]any{
					"name":  server.Name,
					"state": state,
					"type":  server.ServerType.Name,
				},
			}

			if !server.Created.IsZero() {
				meta.Attr["createdAt"] = server.Created
				earliest = preferEarlier(earliest, &server.Created)
			}

			if details.Summary.Region == "" {
				details.Summary.Region = server.Datacenter.Location.Name
			}

			switch state {
			case StateActive:
				hasActive = true
			case StateStopped:
				hasStopped = true
			}

			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover volumes by label
	volumes, err := client.Volume.AllWithOpts(ctx, hcloud.VolumeListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: fmt.Sprintf("%s=%s", deploymentIDLabel, deploymentID),
		},
	})
	if err != nil {
		slog.Debug("list volumes failed", "error", err)
	} else {
		for _, vol := range volumes {
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    volumeARN(vol.ID),
					Type:   ResourceVolume,
					Region: vol.Location.Name,
					ID:     strconv.FormatInt(vol.ID, 10),
				},
				Tags: vol.Labels,
				Attr: map[string]any{
					"name": vol.Name,
					"size": vol.Size,
				},
			}

			if !vol.Created.IsZero() {
				meta.Attr["createdAt"] = vol.Created
				earliest = preferEarlier(earliest, &vol.Created)
			}

			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover networks by label
	networks, err := client.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: fmt.Sprintf("%s=%s", deploymentIDLabel, deploymentID),
		},
	})
	if err != nil {
		slog.Debug("list networks failed", "error", err)
	} else {
		for _, net := range networks {
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    networkARN(net.ID),
					Type:   ResourceNetwork,
					Region: "",
					ID:     strconv.FormatInt(net.ID, 10),
				},
				Tags: net.Labels,
				Attr: map[string]any{
					"name": net.Name,
				},
			}

			if !net.Created.IsZero() {
				meta.Attr["createdAt"] = net.Created
				earliest = preferEarlier(earliest, &net.Created)
			}

			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover firewalls by label
	firewalls, err := client.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: fmt.Sprintf("%s=%s", deploymentIDLabel, deploymentID),
		},
	})
	if err != nil {
		slog.Debug("list firewalls failed", "error", err)
	} else {
		for _, fw := range firewalls {
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    firewallARN(fw.ID),
					Type:   ResourceFirewall,
					Region: "",
					ID:     strconv.FormatInt(fw.ID, 10),
				},
				Tags: fw.Labels,
				Attr: map[string]any{
					"name": fw.Name,
				},
			}

			if !fw.Created.IsZero() {
				meta.Attr["createdAt"] = fw.Created
				earliest = preferEarlier(earliest, &fw.Created)
			}

			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover SSH keys by label
	sshKeys, err := client.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: fmt.Sprintf("%s=%s", deploymentIDLabel, deploymentID),
		},
	})
	if err != nil {
		slog.Debug("list ssh keys failed", "error", err)
	} else {
		for _, key := range sshKeys {
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    sshKeyARN(key.ID),
					Type:   ResourceSSHKey,
					Region: "",
					ID:     strconv.FormatInt(key.ID, 10),
				},
				Tags: key.Labels,
				Attr: map[string]any{
					"name": key.Name,
				},
			}

			if !key.Created.IsZero() {
				meta.Attr["createdAt"] = key.Created
				earliest = preferEarlier(earliest, &key.Created)
			}

			details.Resources = append(details.Resources, meta)
		}
	}

	// Update summary
	details.Summary.Resources = len(details.Resources)
	if earliest != nil {
		details.Summary.CreatedAt = *earliest
	}

	// Determine state
	if hasServers {
		switch {
		case hasActive:
			details.Summary.State = StateActive
		case hasStopped:
			details.Summary.State = StateStopped
		case details.Summary.Resources > 0:
			details.Summary.State = StateTerminated
		}
	} else if details.Summary.Resources > 0 {
		details.Summary.State = "orphaned"
	}

	if details.Summary.Owner == "" {
		details.Summary.Owner = "-"
	}

	return details, nil
}

// CollectDeploymentSummaries discovers deployments across Hetzner Cloud
func CollectDeploymentSummaries(
	ctx context.Context,
	ownerFilter string,
) ([]DeploymentSummary, error) {
	client := newHCloudClient()

	summaries := make(map[string]*DeploymentSummary)
	deploymentIDRegex := regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)

	// Discover deployments via servers (primary resource)
	servers, err := client.Server.AllWithOpts(ctx, hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "project=exasol-personal",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	for _, server := range servers {
		depID := extractDeploymentIDFromLabels(server.Labels, deploymentIDRegex)
		if depID == "" {
			depID = extractDeploymentIDFromName(server.Name, deploymentIDRegex)
		}
		if depID == "" {
			continue
		}

		sum := summaries[depID]
		if sum == nil {
			sum = &DeploymentSummary{
				ID:        depID,
				Provider:  ProviderName,
				Region:    server.Datacenter.Location.Name,
				Owner:     "-",
				CreatedAt: time.Time{},
				State:     StateUnknown,
			}
			summaries[depID] = sum
		}

		sum.Resources++

		if !server.Created.IsZero() && (sum.CreatedAt.IsZero() || server.Created.Before(sum.CreatedAt)) {
			sum.CreatedAt = server.Created
		}

		state := serverStateToSimple(server.Status)
		switch state {
		case StateActive:
			if sum.State != StateActive {
				sum.State = StateActive
			}
		case StateStopped:
			if sum.State != StateActive {
				sum.State = StateStopped
			}
		}
	}

	// Count volumes for each deployment
	volumes, err := client.Volume.AllWithOpts(ctx, hcloud.VolumeListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "project=exasol-personal",
		},
	})
	if err != nil {
		slog.Debug("list volumes failed", "error", err)
	}
	for _, vol := range volumes {
		depID := extractDeploymentIDFromLabels(vol.Labels, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	// Count networks for each deployment
	networks, err := client.Network.AllWithOpts(ctx, hcloud.NetworkListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "project=exasol-personal",
		},
	})
	if err != nil {
		slog.Debug("list networks failed", "error", err)
	}
	for _, net := range networks {
		depID := extractDeploymentIDFromLabels(net.Labels, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	// Count firewalls for each deployment
	firewalls, err := client.Firewall.AllWithOpts(ctx, hcloud.FirewallListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "project=exasol-personal",
		},
	})
	if err != nil {
		slog.Debug("list firewalls failed", "error", err)
	}
	for _, fw := range firewalls {
		depID := extractDeploymentIDFromLabels(fw.Labels, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	// Count SSH keys for each deployment
	sshKeys, err := client.SSHKey.AllWithOpts(ctx, hcloud.SSHKeyListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: "project=exasol-personal",
		},
	})
	if err != nil {
		slog.Debug("list ssh keys failed", "error", err)
	}
	for _, key := range sshKeys {
		depID := extractDeploymentIDFromLabels(key.Labels, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	// Convert map to slice
	result := make([]DeploymentSummary, 0, len(summaries))
	for _, s := range summaries {
		result = append(result, *s)
	}

	return result, nil
}

// Helper functions

func serverStateToSimple(status hcloud.ServerStatus) string {
	switch status {
	case hcloud.ServerStatusRunning:
		return StateActive
	case hcloud.ServerStatusInitializing:
		return StateProvisioning
	case hcloud.ServerStatusOff:
		return StateStopped
	case hcloud.ServerStatusDeleting:
		return StateTerminated
	default:
		return StateUnknown
	}
}

func extractDeploymentIDFromLabels(labels map[string]string, regex *regexp.Regexp) string {
	if labels == nil {
		return ""
	}
	if depID, ok := labels[deploymentIDLabel]; ok && regex.MatchString(depID) {
		return depID
	}
	return ""
}

func extractDeploymentIDFromName(name string, regex *regexp.Regexp) string {
	// Pattern: exasol-{deployment_id}-suffix or exasol-{deployment_id}
	parts := strings.Split(name, "-")
	if len(parts) >= 2 {
		candidate := parts[0] + "-" + parts[1]
		if regex.MatchString(candidate) {
			return candidate
		}
	}

	return ""
}


func preferEarlier(existing *time.Time, candidate *time.Time) *time.Time {
	if candidate == nil {
		return existing
	}
	if existing == nil || candidate.Before(*existing) {
		return candidate
	}
	return existing
}

// ARN generators for Hetzner resources
func serverARN(id int64) string {
	return fmt.Sprintf("hetzner:server:%d", id)
}

func volumeARN(id int64) string {
	return fmt.Sprintf("hetzner:volume:%d", id)
}

func networkARN(id int64) string {
	return fmt.Sprintf("hetzner:network:%d", id)
}

func firewallARN(id int64) string {
	return fmt.Sprintf("hetzner:firewall:%d", id)
}

func sshKeyARN(id int64) string {
	return fmt.Sprintf("hetzner:ssh-key:%d", id)
}
