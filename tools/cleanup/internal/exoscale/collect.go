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
	v3 "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"

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
func createExoscaleClient(ctx context.Context, zone string) (*v3.Client, error) {
	apiKey := os.Getenv("EXOSCALE_API_KEY")
	apiSecret := os.Getenv("EXOSCALE_API_SECRET")
	
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("EXOSCALE_API_KEY and EXOSCALE_API_SECRET environment variables are required")
	}

	creds := credentials.NewStaticCredentials(apiKey, apiSecret)
	
	// Get the zone endpoint
	endpoint, err := getZoneEndpoint(zone)
	if err != nil {
		return nil, err
	}
	
	client, err := v3.NewClient(creds, v3.ClientOptWithEndpoint(endpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to create Exoscale client: %w", err)
	}

	return client, nil
}

// getZoneEndpoint returns the appropriate API endpoint for a zone
func getZoneEndpoint(zone string) (v3.Endpoint, error) {
	switch zone {
	case "ch-gva-2":
		return v3.CHGva2, nil
	case "ch-dk-2":
		return v3.CHDk2, nil
	case "de-fra-1":
		return v3.DEFra1, nil
	case "de-muc-1":
		return v3.DEMuc1, nil
	case "at-vie-1":
		return v3.ATVie1, nil
	case "at-vie-2":
		return v3.ATVie2, nil
	case "bg-sof-1":
		return v3.BGSof1, nil
	default:
		return "", fmt.Errorf("unsupported zone: %s", zone)
	}
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
	instancesResp, err := client.ListInstances(ctx)
	if err != nil {
		slog.Debug("list instances failed", "error", err)
	} else if instancesResp != nil && instancesResp.Instances != nil {
		for _, inst := range instancesResp.Instances {
			// inst.ID is v3.UUID which is a string type
			if inst.ID == "" {
				continue
			}
			instID := inst.ID
			
			// Get full instance to access all fields
			fullInst, err := client.GetInstance(ctx, instID)
			if err != nil {
				slog.Debug("failed to get instance details", "error", err)
				continue
			}
			
			nameStr := fullInst.Name
			labels := fullInst.Labels
			
			if !matchesDeployment(&nameStr, labels, deploymentID) {
				continue
			}
			
			hasInstances = true
			stateStr := string(fullInst.State)
			state := instanceStateToSimple(&stateStr)
			
			typeID := ""
			if fullInst.InstanceType != nil {
				typeID = string(fullInst.InstanceType.ID)
			}
			
			instIDStr := string(instID)
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    instanceARN(zone, instIDStr),
					Type:   ResourceComputeInstance,
					Region: zone,
					ID:     instIDStr,
				},
				Tags: labels,
				Attr: map[string]any{
					"name":  nameStr,
					"state": state,
					"type":  typeID,
				},
			}
			
			if !fullInst.CreatedAT.IsZero() {
				meta.Attr["createdAt"] = fullInst.CreatedAT
				earliest = preferEarlier(earliest, &fullInst.CreatedAT)
			}
			
			if owner, ok := labels["owner"]; ok && owner != "" {
				details.Summary.Owner = owner
			}
			
			switch state {
			case StateActive:
				hasActive = true
			case StateStopped:
				hasStopped = true
			}
		}
	}

	// Block storage volumes
	volumesResp, err := client.ListBlockStorageVolumes(ctx)
	if err != nil {
		slog.Debug("list block storage volumes failed", "error", err)
	} else if volumesResp != nil && volumesResp.BlockStorageVolumes != nil {
		for _, vol := range volumesResp.BlockStorageVolumes {
			labels := vol.Labels
			
			if !matchesDeploymentLabels(labels, deploymentID) {
				continue
			}
			
			volID := string(vol.ID)
			volName := vol.Name
			stateStr := string(vol.State)
			state := blockStorageStateToSimple(stateStr)
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    volumeARN(zone, volID),
					Type:   ResourceBlockVolume,
					Region: zone,
					ID:     volID,
				},
				Tags: labels,
				Attr: map[string]any{
					"name":  volName,
					"state": state,
					"size":  vol.Size,
				},
			}
			
			if !vol.CreatedAT.IsZero() {
				meta.Attr["createdAt"] = vol.CreatedAT
				earliest = preferEarlier(earliest, &vol.CreatedAT)
			}
			
			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover private networks by label
	networksResp, err := client.ListPrivateNetworks(ctx)
	if err != nil {
		slog.Debug("list private networks failed", "error", err)
	} else if networksResp != nil && networksResp.PrivateNetworks != nil {
		for _, net := range networksResp.PrivateNetworks {
			nameStr := net.Name
			labels := net.Labels
			
			if !matchesDeployment(&nameStr, labels, deploymentID) {
				continue
			}
			
			netID := string(net.ID)
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    networkARN(zone, netID),
					Type:   ResourcePrivateNetwork,
					Region: zone,
					ID:     netID,
				},
				Tags: labels,
				Attr: map[string]any{
					"name": nameStr,
				},
			}
			
			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover security groups by name pattern
	securityGroupsResp, err := client.ListSecurityGroups(ctx)
	if err != nil {
		slog.Debug("list security groups failed", "error", err)
	} else if securityGroupsResp != nil && securityGroupsResp.SecurityGroups != nil {
		for _, sg := range securityGroupsResp.SecurityGroups {
			nameStr := sg.Name
			
			if !matchesDeploymentName(&nameStr, deploymentID) {
				continue
			}
			
			sgID := string(sg.ID)
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    securityGroupARN(zone, sgID),
					Type:   ResourceSecurityGroup,
					Region: zone,
					ID:     sgID,
				},
				Tags: map[string]string{},
				Attr: map[string]any{
					"name": nameStr,
				},
			}
			
			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover SSH keys by name pattern
	sshKeysResp, err := client.ListSSHKeys(ctx)
	if err != nil {
		slog.Debug("list ssh keys failed", "error", err)
	} else if sshKeysResp != nil && sshKeysResp.SSHKeys != nil {
		for _, key := range sshKeysResp.SSHKeys {
			nameStr := key.Name
			
			if !matchesDeploymentName(&nameStr, deploymentID) {
				continue
			}
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    sshKeyARN(zone, nameStr),
					Type:   ResourceSSHKey,
					Region: zone,
					ID:     nameStr,
				},
				Tags: map[string]string{},
				Attr: map[string]any{
					"name": nameStr,
				},
			}
			
			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover IAM roles by label
	iamRolesResp, err := client.ListIAMRoles(ctx)
	if err != nil {
		slog.Debug("list iam roles failed", "error", err)
	} else if iamRolesResp != nil && iamRolesResp.IAMRoles != nil {
		for _, role := range iamRolesResp.IAMRoles {
			nameStr := role.Name
			labels := role.Labels
			
			if !matchesDeployment(&nameStr, labels, deploymentID) {
				continue
			}
			
			roleID := string(role.ID)
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    iamRoleARN(roleID),
					Type:   ResourceIAMRole,
					Region: zone,
					ID:     roleID,
				},
				Tags: labels,
				Attr: map[string]any{
					"name": nameStr,
				},
			}
			
			details.Resources = append(details.Resources, meta)
		}
	}

	// Discover IAM API keys
	apiKeysResp, err := client.ListAPIKeys(ctx)
	if err != nil {
		slog.Debug("list iam api keys failed", "error", err)
	} else if apiKeysResp != nil && apiKeysResp.APIKeys != nil {
		for _, key := range apiKeysResp.APIKeys {
			nameStr := key.Name
			
			if !matchesDeploymentName(&nameStr, deploymentID) {
				continue
			}
			
			keyStr := key.Key
			
			meta := ResourceMeta{
				Ref: ResourceRef{
					ARN:    iamAPIKeyARN(keyStr),
					Type:   ResourceIAMAPIKey,
					Region: zone,
					ID:     keyStr,
				},
				Tags: map[string]string{},
				Attr: map[string]any{
					"name": nameStr,
				},
			}
			
			details.Resources = append(details.Resources, meta)
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
	instancesResp, err := client.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	if instancesResp != nil && instancesResp.Instances != nil {
		for _, inst := range instancesResp.Instances {
			depID := extractDeploymentID(&inst.Name, inst.Labels, deploymentIDRegex)
			if depID == "" {
				continue
			}

			owner := inst.Labels["owner"]

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

			if !inst.CreatedAT.IsZero() && (sum.CreatedAt.IsZero() || inst.CreatedAT.Before(sum.CreatedAt)) {
				sum.CreatedAt = inst.CreatedAT
			}

			stateStr := string(inst.State)
			state := instanceStateToSimple(&stateStr)
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
	}

	// Block storage volumes
	volumesResp, _ := client.ListBlockStorageVolumes(ctx)
	if volumesResp != nil && volumesResp.BlockStorageVolumes != nil {
		for _, vol := range volumesResp.BlockStorageVolumes {
			depID := extractDeploymentIDFromLabels(vol.Labels, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}
	
	// Count other resources for each deployment
	networksResp, _ := client.ListPrivateNetworks(ctx)
	if networksResp != nil && networksResp.PrivateNetworks != nil {
		for _, net := range networksResp.PrivateNetworks {
			depID := extractDeploymentID(&net.Name, net.Labels, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}

	securityGroupsResp, _ := client.ListSecurityGroups(ctx)
	if securityGroupsResp != nil && securityGroupsResp.SecurityGroups != nil {
		for _, sg := range securityGroupsResp.SecurityGroups {
			depID := extractDeploymentIDFromName(&sg.Name, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}

	sshKeysResp, _ := client.ListSSHKeys(ctx)
	if sshKeysResp != nil && sshKeysResp.SSHKeys != nil {
		for _, key := range sshKeysResp.SSHKeys {
			depID := extractDeploymentIDFromName(&key.Name, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}

	iamRolesResp, _ := client.ListIAMRoles(ctx)
	if iamRolesResp != nil && iamRolesResp.IAMRoles != nil {
		for _, role := range iamRolesResp.IAMRoles {
			depID := extractDeploymentID(&role.Name, role.Labels, deploymentIDRegex)
			if sum, ok := summaries[depID]; ok {
				sum.Resources++
			}
		}
	}

	apiKeysResp, _ := client.ListAPIKeys(ctx)
	if apiKeysResp != nil && apiKeysResp.APIKeys != nil {
		for _, key := range apiKeysResp.APIKeys {
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
