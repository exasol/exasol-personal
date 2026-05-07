// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/stackitcloud/stackit-sdk-go/core/config"
	iaas "github.com/stackitcloud/stackit-sdk-go/services/iaas/v2api"
	objectstorage "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api"
	resourcemanager "github.com/stackitcloud/stackit-sdk-go/services/resourcemanager/v0api"

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

// createStackitClient creates a STACKIT client using environment credentials
func createStackitClient(ctx context.Context) (*iaas.APIClient, *objectstorage.APIClient, *resourcemanager.APIClient, error) {

	keyPath := os.Getenv("STACKIT_SERVICE_ACCOUNT_KEY_PATH")

	if keyPath == "" {
		return nil, nil, nil, fmt.Errorf("STACKIT_SERVICE_ACCOUNT_KEY_PATH environment variable is required")
	}

	config := config.WithServiceAccountKeyPath(keyPath)

	iaasClient, err := iaas.NewAPIClient(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create STACKIT IaaS client: %w", err)
	}

	objectStorageClient, err := objectstorage.NewAPIClient(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create STACKIT object storage client: %w", err)
	}

	resourceManagerClient, err := resourcemanager.NewAPIClient(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create STACKIT object storage client: %w", err)
	}

	return iaasClient, objectStorageClient, resourceManagerClient, nil
}

func GetAccountInfo(ctx context.Context, projectId string) (string, error) {
	_, _, resourceManagerClient, err := createStackitClient(ctx)
	if err != nil {
		return "", err
	}

	projectResp, err := resourceManagerClient.DefaultAPI.GetProject(ctx, projectId).Execute()
	if err != nil {
		return projectResp.GetName(), nil
	}

	return "[restricted]", nil

}

func CollectResources(ctx context.Context, projectId, region string, deploymentId *string) ([]ResourceMeta, error) {
	resources := []ResourceMeta{}

	iaasClient, objectStorageClient, _, err := createStackitClient(ctx)
	if err != nil {
		return nil, err
	}

	serversResp, err := iaasClient.DefaultAPI.ListServers(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list servers failed", "error", err)
	} else {
		for _, server := range serversResp.GetItems() {
			meta, err := ResourceMetaFromServer(server, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	volumesResp, err := iaasClient.DefaultAPI.ListVolumes(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list volumes failed", "error", err)
	} else {
		for _, vol := range volumesResp.GetItems() {
			meta, err := ResourceMetaFromVolume(vol, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	networksResp, err := iaasClient.DefaultAPI.ListNetworks(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list private networks failed", "error", err)
	} else {
		for _, net := range networksResp.GetItems() {
			meta, err := ResourceMetaFromNetwork(net, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	securityGroupsResp, err := iaasClient.DefaultAPI.ListSecurityGroups(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list security groups failed", "error", err)
	} else {
		for _, sg := range securityGroupsResp.GetItems() {
			meta, err := ResourceMetaFromSecurityGroup(sg, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	// Discover buckets by name pattern
	bucketsResp, err := objectStorageClient.DefaultAPI.ListBuckets(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list object storage buckets failed", "error", err)
	} else {
		for _, bucket := range bucketsResp.GetBuckets() {
			meta, err := ResourceMetaFromBucket(bucket, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	credsResp, err := objectStorageClient.DefaultAPI.ListAccessKeys(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list object storage credentials failed", "error", err)
	} else {
		for _, cred := range credsResp.GetAccessKeys() {
			meta, err := ResourceMetaFromAccessKey(cred, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	cgResp, err := objectStorageClient.DefaultAPI.ListCredentialsGroups(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list object storage credentials group failed", "error", err)
	} else {
		for _, cg := range cgResp.GetCredentialsGroups() {
			meta, err := ResourceMetaFromCredentialsGroup(cg, projectId, region)
			if err != nil {
				return nil, err
			}

			if isDeployment(meta, deploymentId) {
				resources = append(resources, *meta)
			}
		}
	}

	return resources, nil
}

// CollectDeploymentDetails enumerates resources for a single deployment in STACKIT
func CollectDeploymentDetails(
	ctx context.Context,
	projectId,
	region,
	deploymentId string,
) (*DeploymentDetails, error) {
	resources, err := CollectResources(ctx, projectId, region, &deploymentId)
	if err != nil {
		return nil, err
	}

	details := &DeploymentDetails{
		Summary: DeploymentSummary{
			ID:        deploymentId,
			Provider:  "stackit",
			Region:    region,
			Owner:     "",
			CreatedAt: time.Time{},
			State:     StateUnknown,
		},
		Resources: resources,
	}

	var earliest *time.Time
	hasInstances := false
	hasActive := false
	hasStopped := false

	for _, meta := range resources {
		if meta.Ref.Type == ResourceServer {
			state := meta.Attr["status"].(string)
			switch state {
			case StateActive:
				hasActive = true
			case StateStopped:
				hasStopped = true
			}
		}

		createdAt, ok := meta.Attr["createdAt"].(time.Time)
		if ok && !createdAt.IsZero() && createdAt.Before(*earliest) {
			earliest = &createdAt
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

// CollectDeploymentSummaries discovers deployments across the STACKIT region
func CollectDeploymentSummaries(
	ctx context.Context,
	projectId,
	region string,
) ([]DeploymentSummary, error) {
	resources, err := CollectResources(ctx, projectId, region, nil)
	if err != nil {
		return nil, err
	}

	summaries := make(map[string]*DeploymentSummary)
	for _, meta := range resources {
		depId, ok := getDeploymentId(&meta)
		if ok {
			sum, ok := summaries[depId]
			if !ok {
				sum = &DeploymentSummary{
					ID:        depId,
					Provider:  "stackit",
					Region:    region,
					Owner:     "",
					CreatedAt: time.Time{},
					State:     StateUnknown,
				}

				summaries[depId] = sum
			}

			sum.Resources++

			if meta.Ref.Type == ResourceServer {
				state := meta.Attr["state"].(string)
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

			createdAt, ok := meta.Attr["createdAt"].(time.Time)
			if ok && !createdAt.IsZero() && createdAt.Before(sum.CreatedAt) {
				sum.CreatedAt = createdAt
			}
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

func ResourceMetaFromServer(server iaas.Server, projectId, region string) (*ResourceMeta, error) {
	labels, err := toStringMap(server.GetLabels())
	if err != nil {
		return nil, err
	}

	id := server.GetId()
	name := server.GetName()
	state := serverStateToSimple(server.GetStatus())
	machineType := server.GetMachineType()
	createdAt := server.GetCreatedAt()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    serverARN(region, projectId, id),
			Type:   ResourceServer,
			Region: region,
			ID:     id,
		},
		Tags: labels,
		Attr: map[string]any{
			"name":      name,
			"state":     state,
			"type":      machineType,
			"createdAt": createdAt,
		},
	}

	return &meta, nil
}

func ResourceMetaFromVolume(vol iaas.Volume, projectId, region string) (*ResourceMeta, error) {
	labels, err := toStringMap(vol.GetLabels())
	if err != nil {
		return nil, err
	}

	id := vol.GetId()
	name := vol.GetName()
	state := volumeStateToSimple(vol.GetStatus())
	size := vol.GetSize()
	createdAt := vol.GetCreatedAt()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    volumeARN(region, projectId, id),
			Type:   ResourceVolume,
			Region: region,
			ID:     id,
		},
		Tags: labels,
		Attr: map[string]any{
			"name":      name,
			"state":     state,
			"size":      size,
			"createdAt": createdAt,
		},
	}

	return &meta, nil
}

func ResourceMetaFromNetwork(net iaas.Network, projectId, region string) (*ResourceMeta, error) {
	labels, err := toStringMap(net.GetLabels())
	if err != nil {
		return nil, err
	}

	id := net.GetId()
	name := net.GetName()
	createdAt := net.GetCreatedAt()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    networkARN(region, projectId, id),
			Type:   ResourceNetwork,
			Region: region,
			ID:     id,
		},
		Tags: labels,
		Attr: map[string]any{
			"name":      name,
			"createdAt": createdAt,
		},
	}

	return &meta, nil
}

func ResourceMetaFromSecurityGroup(sg iaas.SecurityGroup, projectId, region string) (*ResourceMeta, error) {
	labels, err := toStringMap(sg.GetLabels())
	if err != nil {
		return nil, err
	}

	id := sg.GetId()
	name := sg.GetName()
	createdAt := sg.GetCreatedAt()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    securityGroupARN(region, projectId, id),
			Type:   ResourceSecurityGroup,
			Region: region,
			ID:     id,
		},
		Tags: labels,
		Attr: map[string]any{
			"name":      name,
			"createdAt": createdAt,
		},
	}

	return &meta, nil
}

func ResourceMetaFromBucket(sg objectstorage.Bucket, projectId, region string) (*ResourceMeta, error) {

	id := sg.GetName()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    bucketARN(region, projectId, id),
			Type:   ResourceObjectStorageBucket,
			Region: region,
			ID:     id,
		},
		Tags: map[string]string{},
		Attr: map[string]any{
			"name": id,
		},
	}

	return &meta, nil
}

func ResourceMetaFromAccessKey(key objectstorage.AccessKey, projectId, region string) (*ResourceMeta, error) {

	id := key.GetKeyId()
	name := key.GetDisplayName()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    credentialARN(region, projectId, id),
			Type:   ResourceObjectStorageCredential,
			Region: region,
			ID:     id,
		},
		Tags: map[string]string{},
		Attr: map[string]any{
			"name": name,
		},
	}

	return &meta, nil
}

func ResourceMetaFromCredentialsGroup(cg objectstorage.CredentialsGroup, projectId, region string) (*ResourceMeta, error) {

	id := cg.GetCredentialsGroupId()
	name := cg.GetDisplayName()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    credentialsGroupARN(region, projectId, id),
			Type:   ResourceObjectStorageCredentialsGroup,
			Region: region,
			ID:     id,
		},
		Tags: map[string]string{},
		Attr: map[string]any{
			"name": name,
		},
	}

	return &meta, nil
}

func isDeployment(meta *ResourceMeta, deploymentId *string) bool {
	if deploymentId == nil {
		return true
	}

	if depId, ok := meta.Tags["Deployment"]; ok && depId == *deploymentId {
		return true
	}

	if name, ok := meta.Attr["name"]; ok && (strings.HasPrefix(name.(string), *deploymentId+"-") || name == *deploymentId) {
		return true
	}

	return false
}

func getDeploymentId(meta *ResourceMeta) (string, bool) {
	if depId, ok := meta.Tags["Deployment"]; ok {
		return depId, true
	}

	if name, ok := meta.Attr["name"].(string); ok {
		// Pattern: exasol-{deployment_id}-suffix or exasol-{deployment_id}
		parts := strings.Split(name, "-")
		if len(parts) >= 2 {
			// Try exasol-XXXXXXXX pattern
			candidate := parts[0] + "-" + parts[1]
			regex := regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)
			if regex.MatchString(candidate) {
				return candidate, true
			}
		}
	}

	return "", false
}

func toStringMap(m map[string]interface{}) (map[string]string, error) {
	result := make(map[string]string)

	for k, v := range m {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("value for key %s is not a string", k)
		}
		result[k] = str
	}

	return result, nil
}

func serverStateToSimple(state string) string {
	switch state {
	case "ACTIVE", "BACKING-UP", "SNAPSHOTTING", "STARTING":
		return StateActive
	case "CREATING", "REBOOTING", "REBUILD", "REBUILDING", "RESCUE", "RESCUING", "RESIZING", "UNRESCUING", "UPDATING":
		return StateProvisioning
	case "DEALLOCATED", "DEALLOCATING", "DELETED", "DELETING":
		return StateTerminated
	case "ERROR", "INACTIVE", "MIGRATING", "PAUSED":
		return StateStopped
	default:
		return StateUnknown
	}
}

func volumeStateToSimple(state string) string {
	switch state {
	case "ATTACHED", "AVAILABLE", "BACKING-UP", "ERROR_BACKING-UP", "ERROR_DELETING", "ERROR_RESIZING", "ERROR_RESTORING-BACKUP":
		return StateActive
	case "ATTACHING", "AWAITING-TRANSFER", "CREATING", "DETACHING", "MAINTENANCE", "RESERVED", "RESIZING", "RESTORING-BACKUP", "RETYPING", "UPLOADING":
		return StateProvisioning
	case "DELETED", "DELETING", "ERROR":
		return StateTerminated
	default:
		return StateUnknown
	}
}

// ARN generators for STACKIT  resources (using similar format to AWS ARNs)
func serverARN(region, projectId, id string) string {
	return fmt.Sprintf("stackit:%s:project:%s:server:%s", region, projectId, id)
}

func volumeARN(region, projectId, id string) string {
	return fmt.Sprintf("stackit:%s:project:%s:volume:%s", region, projectId, id)
}

func networkARN(region, projectId, id string) string {
	return fmt.Sprintf("stackit:%s:project:%s:network:%s", region, projectId, id)
}

func securityGroupARN(region, projectId, id string) string {
	return fmt.Sprintf("stackit:%s:project:%s:security-group:%s", region, projectId, id)
}

func bucketARN(region, projectId, bucket string) string {
	return fmt.Sprintf("stackit:%s:project:%s:bucket:%s", region, projectId, bucket)
}

func credentialARN(region, projectId, credential string) string {
	return fmt.Sprintf("stackit:%s:project:%s:credential:%s", region, projectId, credential)
}

func credentialsGroupARN(region, projectId, cg string) string {
	return fmt.Sprintf("stackit:%s:project:%s:credentials-group:%s", region, projectId, cg)
}
