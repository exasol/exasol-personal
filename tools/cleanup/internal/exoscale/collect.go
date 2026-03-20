// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	egoscale "github.com/exoscale/egoscale/v2"

	"github.com/exasol/exasol-personal/tools/cleanup/internal/shared"
)

// Constants from shared package
const (
	StateActive       = shared.StateActive
	StateProvisioning = shared.StateProvisioning
	StateStopped      = shared.StateStopped
	StateTerminated   = shared.StateTerminated
	StateUnknown      = shared.StateUnknown
)

// createExoscaleClient creates an Exoscale client using environment credentials
func createExoscaleClient(ctx context.Context, zone string) (*egoscale.Client, error) {
	apiKey := os.Getenv("EXOSCALE_API_KEY")
	apiSecret := os.Getenv("EXOSCALE_API_SECRET")
	
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("EXOSCALE_API_KEY and EXOSCALE_API_SECRET environment variables are required")
	}

	client, err := egoscale.NewClient(
		apiKey,
		apiSecret,
		egoscale.ClientOptWithAPIEndpoint(fmt.Sprintf("https://api-%s.exoscale.com/v2", zone)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Exoscale client: %w", err)
	}

	return client, nil
}

// CollectDeploymentDetails enumerates resources for a single deployment in Exoscale
func CollectDeploymentDetails(
	ctx context.Context,
	zone string,
	deploymentID string,
) (*DeploymentDetails, error) {
	client, err := createExoscaleClient(ctx, zone)
	if err != nil {
		return nil, err
	}

	details := &DeploymentDetails{
		Summary: DeploymentSummary{
			ID:        deploymentID,
			Provider:  "exoscale",
			Region:    zone,
			Owner:     "",
			CreatedAt: time.Time{},
			State:     StateUnknown,
		},
	}

	var earliest *time.Time
	hasInstances := false
	hasActive := false
	hasStopped := false

	// Discover compute instances by label
	instances, err := client.ListInstances(ctx, zone)
	if err != nil {
		slog.Debug("list instances failed", "error", err)
	} else {
		for _, inst := range instances {
			if matchesDeployment(inst.Name, derefLabels(inst.Labels), deploymentID) {
				hasInstances = true
				state := instanceStateToSimple(inst.State)
				
				meta := ResourceMeta{
					Ref: ResourceRef{
						ARN:    instanceARN(zone, ptrString(inst.ID)),
						Type:   ResourceComputeInstance,
						Region: zone,
						ID:     ptrString(inst.ID),
					},
					Tags: derefLabels(inst.Labels),
					Attr: map[string]any{
						"name":  ptrString(inst.Name),
						"state": state,
						"type":  ptrString(inst.InstanceTypeID),
					},
				}
				
				if inst.CreatedAt != nil {
					meta.Attr["createdAt"] = *inst.CreatedAt
					earliest = preferEarlier(earliest, inst.CreatedAt)
				}
				
				labels := derefLabels(inst.Labels)
				if owner, ok := labels["owner"]; ok && owner != "" {
					if details.Summary.Owner == "" {
						details.Summary.Owner = owner
					}
				}
				
				details.Resources = append(details.Resources, meta)
				
				switch state {
				case StateActive:
					hasActive = true
				case StateStopped:
					hasStopped = true
				}
			}
		}
	}

	// Block storage volumes - use direct API client
	apiCli, err := newAPIClient(zone)
	if err == nil {
		volumes, err := apiCli.listBlockStorageVolumes(ctx)
		if err != nil {
			slog.Debug("list block storage volumes failed", "error", err)
		} else {
			for _, vol := range volumes {
				if matchesDeploymentLabels(vol.Labels, deploymentID) {
					state := blockStorageStateToSimple(vol.State)
					
					meta := ResourceMeta{
						Ref: ResourceRef{
							ARN:    volumeARN(zone, vol.ID),
							Type:   ResourceBlockVolume,
							Region: zone,
							ID:     vol.ID,
						},
						Tags: vol.Labels,
						Attr: map[string]any{
							"name":  vol.Name,
							"state": state,
							"size":  vol.Size,
						},
					}
					
					if vol.CreatedAt != "" {
						if createdAt, err := time.Parse(time.RFC3339, vol.CreatedAt); err == nil {
							meta.Attr["createdAt"] = createdAt
							earliest = preferEarlier(earliest, &createdAt)
						}
					}
					
					details.Resources = append(details.Resources, meta)
				}
			}
		}
	}

	// Discover private networks by label
	networks, err := client.ListPrivateNetworks(ctx, zone)
	if err != nil {
		slog.Debug("list private networks failed", "error", err)
	} else {
		for _, net := range networks {
			if matchesDeployment(net.Name, derefLabels(net.Labels), deploymentID) {
				meta := ResourceMeta{
					Ref: ResourceRef{
						ARN:    networkARN(zone, ptrString(net.ID)),
						Type:   ResourcePrivateNetwork,
						Region: zone,
						ID:     ptrString(net.ID),
					},
					Tags: derefLabels(net.Labels),
					Attr: map[string]any{
						"name": ptrString(net.Name),
					},
				}
				
				details.Resources = append(details.Resources, meta)
			}
		}
	}

	// Discover security groups by name pattern
	securityGroups, err := client.ListSecurityGroups(ctx, zone)
	if err != nil {
		slog.Debug("list security groups failed", "error", err)
	} else {
		for _, sg := range securityGroups {
			if matchesDeploymentName(sg.Name, deploymentID) {
				meta := ResourceMeta{
					Ref: ResourceRef{
						ARN:    securityGroupARN(zone, ptrString(sg.ID)),
						Type:   ResourceSecurityGroup,
						Region: zone,
						ID:     ptrString(sg.ID),
					},
					Tags: map[string]string{},
					Attr: map[string]any{
						"name": ptrString(sg.Name),
					},
				}
				
				details.Resources = append(details.Resources, meta)
			}
		}
	}

	// Discover SSH keys by name pattern
	sshKeys, err := client.ListSSHKeys(ctx, zone)
	if err != nil {
		slog.Debug("list ssh keys failed", "error", err)
	} else {
		for _, key := range sshKeys {
			if matchesDeploymentName(key.Name, deploymentID) {
				meta := ResourceMeta{
					Ref: ResourceRef{
						ARN:    sshKeyARN(zone, ptrString(key.Name)),
						Type:   ResourceSSHKey,
						Region: zone,
						ID:     ptrString(key.Name),
					},
					Tags: map[string]string{},
					Attr: map[string]any{
						"name": ptrString(key.Name),
					},
				}
				
				details.Resources = append(details.Resources, meta)
			}
		}
	}

	// Discover IAM roles by label
	iamRoles, err := client.ListIAMRoles(ctx, zone)
	if err != nil {
		slog.Debug("list iam roles failed", "error", err)
	} else {
		for _, role := range iamRoles {
			if matchesDeployment(role.Name, role.Labels, deploymentID) {
				meta := ResourceMeta{
					Ref: ResourceRef{
						ARN:    iamRoleARN(ptrString(role.ID)),
						Type:   ResourceIAMRole,
						Region: zone,
						ID:     ptrString(role.ID),
					},
					Tags: role.Labels,
					Attr: map[string]any{
						"name": ptrString(role.Name),
					},
				}
				
				details.Resources = append(details.Resources, meta)
			}
		}
	}

	// Discover IAM API keys - use direct API client
	if apiCli != nil {
		apiKeys, err := apiCli.listIAMAPIKeys(ctx)
		if err != nil {
			slog.Debug("list iam api keys failed", "error", err)
		} else {
			for _, key := range apiKeys {
				if matchesDeploymentName(&key.Name, deploymentID) {
					meta := ResourceMeta{
						Ref: ResourceRef{
							ARN:    iamAPIKeyARN(key.Key),
							Type:   ResourceIAMAPIKey,
							Region: zone,
							ID:     key.Key,
						},
						Tags: map[string]string{},
						Attr: map[string]any{
							"name": key.Name,
						},
					}
					
					details.Resources = append(details.Resources, meta)
				}
			}
		}
	}

	// Discover SOS buckets by name pattern
	sosBuckets, err := listSOSBuckets(ctx, zone, deploymentID)
	if err != nil {
		slog.Debug("list sos buckets failed", "error", err)
	} else {
		for _, bucket := range sosBuckets {
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    sosBucketARN(zone, bucket),
					Type:   ResourceSOSBucket,
					Region: zone,
					ID:     bucket,
				},
				Tags: map[string]string{},
				Attr: map[string]any{
					"name": bucket,
				},
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
	if hasInstances {
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

// CollectDeploymentSummaries discovers deployments across the Exoscale zone
func CollectDeploymentSummaries(
	ctx context.Context,
	zone string,
	ownerFilter string,
	legacy bool,
) ([]DeploymentSummary, error) {
	client, err := createExoscaleClient(ctx, zone)
	if err != nil {
		return nil, err
	}

	summaries := make(map[string]*DeploymentSummary)
	deploymentIDRegex := regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)

	// Discover deployments via compute instances (primary resource)
	instances, err := client.ListInstances(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	for _, inst := range instances {
		depID := extractDeploymentID(inst.Name, derefLabels(inst.Labels), deploymentIDRegex)
		if depID == "" {
			continue
		}

		labels := derefLabels(inst.Labels)
		owner := labels["owner"]

		if !ownerMatchesFilter(owner, ownerFilter) {
			continue
		}

		sum := summaries[depID]
		if sum == nil {
			sum = &DeploymentSummary{
				ID:        depID,
				Provider:  "exoscale",
				Region:    zone,
				Owner:     owner,
				CreatedAt: time.Time{},
				State:     StateUnknown,
			}
			summaries[depID] = sum
		}

		sum.Resources++

		if inst.CreatedAt != nil && (sum.CreatedAt.IsZero() || inst.CreatedAt.Before(sum.CreatedAt)) {
			sum.CreatedAt = *inst.CreatedAt
		}

		state := instanceStateToSimple(inst.State)
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

	// Block storage volumes - use direct API client
	apiCli, _ := newAPIClient(zone)
	if apiCli != nil {
		volumes, _ := apiCli.listBlockStorageVolumes(ctx)
		for _, vol := range volumes {
			depID := extractDeploymentIDFromLabels(vol.Labels, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}
	
	// Count other resources for each deployment
	networks, _ := client.ListPrivateNetworks(ctx, zone)
	for _, net := range networks {
		depID := extractDeploymentID(net.Name, derefLabels(net.Labels), deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	securityGroups, _ := client.ListSecurityGroups(ctx, zone)
	for _, sg := range securityGroups {
		depID := extractDeploymentIDFromName(sg.Name, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	sshKeys, _ := client.ListSSHKeys(ctx, zone)
	for _, key := range sshKeys {
		depID := extractDeploymentIDFromName(key.Name, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	iamRoles, _ := client.ListIAMRoles(ctx, zone)
	for _, role := range iamRoles {
		depID := extractDeploymentID(role.Name, role.Labels, deploymentIDRegex)
		if sum, ok := summaries[depID]; ok {
			sum.Resources++
		}
	}

	if apiCli != nil {
		apiKeys, _ := apiCli.listIAMAPIKeys(ctx)
		for _, key := range apiKeys {
			depID := extractDeploymentIDFromName(&key.Name, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}
	
	// SOS buckets
	for depID := range summaries {
		buckets, _ := listSOSBuckets(ctx, zone, depID)
		if sum, ok := summaries[depID]; ok {
			sum.Resources += len(buckets)
		}
	}

	// Convert map to slice
	result := make([]DeploymentSummary, 0, len(summaries))
	for _, s := range summaries {
		if s.Owner == "" {
			s.Owner = "-"
		}
		result = append(result, *s)
	}

	return result, nil
}

// Helper functions

func matchesDeployment(name *string, labels map[string]string, deploymentID string) bool {
	// Check labels first
	if labels != nil {
		if depID, ok := labels["deployment_id"]; ok && depID == deploymentID {
			return true
		}
	}
	
	// Check name pattern
	return matchesDeploymentName(name, deploymentID)
}

func matchesDeploymentName(name *string, deploymentID string) bool {
	if name == nil {
		return false
	}
	return strings.HasPrefix(*name, deploymentID+"-") || *name == deploymentID
}

func extractDeploymentID(name *string, labels map[string]string, regex *regexp.Regexp) string {
	// Check label first
	if labels != nil {
		if depID, ok := labels["deployment_id"]; ok && regex.MatchString(depID) {
			return depID
		}
	}
	
	// Check name pattern
	return extractDeploymentIDFromName(name, regex)
}

func extractDeploymentIDFromName(name *string, regex *regexp.Regexp) string {
	if name == nil {
		return ""
	}
	
	// Pattern: exasol-{deployment_id}-suffix or exasol-{deployment_id}
	parts := strings.Split(*name, "-")
	if len(parts) >= 2 {
		// Try exasol-XXXXXXXX pattern
		candidate := parts[0] + "-" + parts[1]
		if regex.MatchString(candidate) {
			return candidate
		}
	}
	
	return ""
}

func matchesDeploymentLabels(labels map[string]string, deploymentID string) bool {
	if labels == nil {
		return false
	}
	if depID, ok := labels["deployment_id"]; ok && depID == deploymentID {
		return true
	}
	return false
}

func extractDeploymentIDFromLabels(labels map[string]string, regex *regexp.Regexp) string {
	if labels == nil {
		return ""
	}
	if depID, ok := labels["deployment_id"]; ok && regex.MatchString(depID) {
		return depID
	}
	return ""
}

func instanceStateToSimple(state *string) string {
	if state == nil {
		return StateUnknown
	}
	
	// Exoscale instance states are strings
	switch *state {
	case "running":
		return StateActive
	case "starting":
		return StateProvisioning
	case "stopped", "stopping":
		return StateStopped
	case "destroyed", "destroying":
		return StateTerminated
	default:
		return StateUnknown
	}
}

func blockStorageStateToSimple(state string) string {
	// Block storage volume states from API
	switch state {
	case "attached", "detached":
		return StateActive
	case "creating", "attaching", "detaching", "snapshotting":
		return StateProvisioning
	case "deleting", "deleted":
		return StateTerminated
	case "error":
		return "error"
	default:
		return StateUnknown
	}
}

// derefLabels safely dereferences a pointer to map[string]string
func derefLabels(labels *map[string]string) map[string]string {
	if labels == nil {
		return map[string]string{}
	}
	return *labels
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

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ownerMatchesFilter(owner, filter string) bool {
	if filter == "" || filter == "*" {
		return true
	}
	pattern := "^" + regexp.QuoteMeta(filter) + "$"
	pattern = strings.ReplaceAll(pattern, "\\*", ".*")
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(owner)
}

// ARN generators for Exoscale resources (using similar format to AWS ARNs)
func instanceARN(zone, id string) string {
	return fmt.Sprintf("exoscale:%s:compute-instance:%s", zone, id)
}

func volumeARN(zone, id string) string {
	return fmt.Sprintf("exoscale:%s:block-storage-volume:%s", zone, id)
}

func networkARN(zone, id string) string {
	return fmt.Sprintf("exoscale:%s:private-network:%s", zone, id)
}

func securityGroupARN(zone, id string) string {
	return fmt.Sprintf("exoscale:%s:security-group:%s", zone, id)
}

func sshKeyARN(zone, name string) string {
	return fmt.Sprintf("exoscale:%s:ssh-key:%s", zone, name)
}

func iamRoleARN(id string) string {
	return fmt.Sprintf("exoscale:global:iam-role:%s", id)
}

func iamAPIKeyARN(key string) string {
	return fmt.Sprintf("exoscale:global:iam-api-key:%s", key)
}

func sosBucketARN(zone, bucket string) string {
	return fmt.Sprintf("exoscale:%s:sos-bucket:%s", zone, bucket)
}

// listIAMAPIKeys is deprecated - use apiClient.listIAMAPIKeys instead
// Keeping this stub for backward compatibility
func listIAMAPIKeys(ctx context.Context, client *egoscale.Client) ([]IAMAPIKey, error) {
	return []IAMAPIKey{}, nil
}

// listSOSBuckets lists SOS buckets matching the deployment pattern
func listSOSBuckets(ctx context.Context, zone, deploymentID string) ([]string, error) {
	// SOS uses S3-compatible API
	sosEndpoint := fmt.Sprintf("https://sos-%s.exo.io", zone)
	
	// Check if we have SOS credentials
	if os.Getenv("EXOSCALE_API_KEY") == "" {
		slog.Debug("SOS bucket discovery skipped: no credentials")
		return []string{}, nil
	}
	
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(zone),
		awsconfig.WithEndpointResolverWithOptions(awssdk.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (awssdk.Endpoint, error) {
				return awssdk.Endpoint{
					URL:               sosEndpoint,
					HostnameImmutable: true,
					SigningRegion:     zone,
				}, nil
			},
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for SOS: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	
	output, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list SOS buckets: %w", err)
	}

	var buckets []string
	for _, bucket := range output.Buckets {
		if bucket.Name != nil && strings.HasPrefix(*bucket.Name, deploymentID) {
			buckets = append(buckets, *bucket.Name)
		}
	}

	return buckets, nil
}
