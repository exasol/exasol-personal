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

	shared "github.com/exasol/exasol-personal/tools/cleanup/pkg/cleanup"
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
	if projectId == "" {
		return "", fmt.Errorf("STACKIT project id is required")
	}

	_, _, resourceManagerClient, err := createStackitClient(ctx)
	if err != nil {
		return "", err
	}

	projectResp, err := resourceManagerClient.DefaultAPI.GetProject(ctx, projectId).Execute()
	if err != nil {
		return "", fmt.Errorf("failed to get STACKIT project %s: %w", projectId, err)
	}

	projectName := projectResp.GetName()
	if projectName == "" {
		return projectId, nil
	}

	return projectName, nil

}

func CollectResources(ctx context.Context, projectId, region string, deploymentId *string) ([]ResourceMeta, error) {
	resources := []ResourceMeta{}

	iaasClient, objectStorageClient, _, err := createStackitClient(ctx)
	if err != nil {
		return nil, err
	}

	servers, err := collectServers(ctx, iaasClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, servers...)

	volumes, err := collectVolumes(ctx, iaasClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, volumes...)

	networks, err := collectNetworks(ctx, iaasClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, networks...)

	securityGroups, err := collectSecurityGroups(ctx, iaasClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, securityGroups...)

	publicIPs, err := collectPublicIPs(ctx, iaasClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, publicIPs...)

	buckets, err := collectBuckets(ctx, objectStorageClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, buckets...)

	accessKeys, err := collectAccessKeys(ctx, objectStorageClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, accessKeys...)

	credentialsGroups, err := collectCredentialsGroups(ctx, objectStorageClient, projectId, region, deploymentId)
	if err != nil {
		return nil, err
	}
	resources = append(resources, credentialsGroups...)

	return resources, nil
}

func collectServers(
	ctx context.Context,
	iaasClient *iaas.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	serversResp, err := iaasClient.DefaultAPI.ListServers(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list servers failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, server := range serversResp.GetItems() {
		meta, err := ResourceMetaFromServer(server, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

func collectVolumes(
	ctx context.Context,
	iaasClient *iaas.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	volumesResp, err := iaasClient.DefaultAPI.ListVolumes(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list volumes failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, vol := range volumesResp.GetItems() {
		meta, err := ResourceMetaFromVolume(vol, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

// collectNetworks also collects the network interfaces of every listed network,
// so that the network listing is only requested once.
func collectNetworks(
	ctx context.Context,
	iaasClient *iaas.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	networksResp, err := iaasClient.DefaultAPI.ListNetworks(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list private networks failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, net := range networksResp.GetItems() {
		meta, err := ResourceMetaFromNetwork(net, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	for _, net := range networksResp.GetItems() {
		nics, err := collectNetworkInterfaces(ctx, iaasClient, projectId, region, net.GetId(), deploymentId)
		if err != nil {
			return nil, err
		}
		resources = append(resources, nics...)
	}

	return resources, nil
}

func collectNetworkInterfaces(
	ctx context.Context,
	iaasClient *iaas.APIClient,
	projectId, region, networkId string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	nicsResp, err := iaasClient.DefaultAPI.ListNics(ctx, projectId, region, networkId).Execute()
	if err != nil {
		slog.Debug("list network interfaces failed", "network_id", networkId, "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, nic := range nicsResp.GetItems() {
		meta, err := ResourceMetaFromNIC(nic, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

func collectSecurityGroups(
	ctx context.Context,
	iaasClient *iaas.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	securityGroupsResp, err := iaasClient.DefaultAPI.ListSecurityGroups(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list security groups failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, sg := range securityGroupsResp.GetItems() {
		meta, err := ResourceMetaFromSecurityGroup(sg, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

func collectPublicIPs(
	ctx context.Context,
	iaasClient *iaas.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	publicIPsResp, err := iaasClient.DefaultAPI.ListPublicIPs(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list public IPs failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, publicIP := range publicIPsResp.GetItems() {
		meta, err := ResourceMetaFromPublicIP(publicIP, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

// collectBuckets discovers buckets by name pattern
func collectBuckets(
	ctx context.Context,
	objectStorageClient *objectstorage.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	bucketsResp, err := objectStorageClient.DefaultAPI.ListBuckets(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list object storage buckets failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, bucket := range bucketsResp.GetBuckets() {
		meta, err := ResourceMetaFromBucket(bucket, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

func collectAccessKeys(
	ctx context.Context,
	objectStorageClient *objectstorage.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	credsResp, err := objectStorageClient.DefaultAPI.ListAccessKeys(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list object storage credentials failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, cred := range credsResp.GetAccessKeys() {
		meta, err := ResourceMetaFromAccessKey(cred, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
		}
	}

	return resources, nil
}

func collectCredentialsGroups(
	ctx context.Context,
	objectStorageClient *objectstorage.APIClient,
	projectId, region string,
	deploymentId *string,
) ([]ResourceMeta, error) {
	cgResp, err := objectStorageClient.DefaultAPI.ListCredentialsGroups(ctx, projectId, region).Execute()
	if err != nil {
		slog.Debug("list object storage credentials group failed", "error", err)

		return nil, nil
	}

	resources := []ResourceMeta{}
	for _, cg := range cgResp.GetCredentialsGroups() {
		meta, err := ResourceMetaFromCredentialsGroup(cg, projectId, region)
		if err != nil {
			return nil, err
		}

		if isDeployment(meta, deploymentId) {
			resources = append(resources, *meta)
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
		Summary:   summarizeDeploymentResources(deploymentId, region, resources),
		Resources: resources,
	}

	return details, nil
}

// CollectDeploymentSummaries discovers deployments across the STACKIT region
func CollectDeploymentSummaries(
	ctx context.Context,
	projectId,
	region,
	ownerFilter string,
) ([]DeploymentSummary, error) {
	resources, err := CollectResources(ctx, projectId, region, nil)
	if err != nil {
		return nil, err
	}

	resourcesByDeployment := make(map[string][]ResourceMeta)
	for _, meta := range resources {
		depId, ok := getDeploymentId(&meta)
		if ok {
			resourcesByDeployment[depId] = append(resourcesByDeployment[depId], meta)
		}
	}

	// Convert map to slice
	result := make([]DeploymentSummary, 0, len(resourcesByDeployment))
	for deploymentID, deploymentResources := range resourcesByDeployment {
		summary := summarizeDeploymentResources(deploymentID, region, deploymentResources)
		if !ownerMatchesFilter(summary.Owner, ownerFilter) {
			continue
		}
		result = append(result, summary)
	}

	return result, nil
}

// Helper functions

type deploymentAccumulator struct {
	owner           string
	earliest        *time.Time
	hasInstances    bool
	hasActive       bool
	hasProvisioning bool
	hasStopped      bool
}

func (acc *deploymentAccumulator) observe(meta ResourceMeta) {
	if acc.owner == "" {
		acc.owner = firstNonEmpty(meta.Tags["Owner"], meta.Tags["owner"])
	}

	if meta.Ref.Type == ResourceServer {
		acc.observeServer(meta)
	}

	if createdAt, ok := meta.Attr["createdAt"].(time.Time); ok && !createdAt.IsZero() {
		acc.earliest = preferEarlierTime(acc.earliest, createdAt)
	}
}

func (acc *deploymentAccumulator) observeServer(meta ResourceMeta) {
	acc.hasInstances = true

	state, ok := meta.Attr["state"].(string)
	if !ok {
		return
	}

	switch state {
	case StateActive:
		acc.hasActive = true
	case StateProvisioning:
		acc.hasProvisioning = true
	case StateStopped:
		acc.hasStopped = true
	}
}

func deploymentState(acc deploymentAccumulator, resourceCount int) string {
	if !acc.hasInstances {
		if resourceCount > 0 {
			return "orphaned"
		}

		return StateUnknown
	}

	switch {
	case acc.hasActive:
		return StateActive
	case acc.hasProvisioning:
		return StateProvisioning
	case acc.hasStopped:
		return StateStopped
	case resourceCount > 0:
		return StateTerminated
	default:
		return StateUnknown
	}
}

func summarizeDeploymentResources(
	deploymentID string,
	region string,
	resources []ResourceMeta,
) DeploymentSummary {
	summary := DeploymentSummary{
		ID:        deploymentID,
		Provider:  "stackit",
		Region:    region,
		Owner:     "",
		CreatedAt: time.Time{},
		State:     StateUnknown,
		Resources: len(resources),
	}

	acc := deploymentAccumulator{}
	for _, meta := range resources {
		acc.observe(meta)
	}

	if acc.earliest != nil {
		summary.CreatedAt = *acc.earliest
	}

	summary.Owner = acc.owner
	summary.State = deploymentState(acc, summary.Resources)

	if summary.Owner == "" {
		summary.Owner = "-"
	}

	return summary
}

func preferEarlierTime(existing *time.Time, candidate time.Time) *time.Time {
	if candidate.IsZero() {
		return existing
	}

	if existing == nil || candidate.Before(*existing) {
		candidateCopy := candidate

		return &candidateCopy
	}

	return existing
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

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

func ResourceMetaFromNIC(nic iaas.NIC, projectId, region string) (*ResourceMeta, error) {
	labels, err := toStringMap(nic.GetLabels())
	if err != nil {
		return nil, err
	}

	id := nic.GetId()
	name := nic.GetName()
	networkID := nic.GetNetworkId()
	device := nic.GetDevice()
	ipv4 := nic.GetIpv4()
	status := strings.ToLower(nic.GetStatus())

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:      nicARN(region, projectId, networkID, id),
			Type:     ResourceNetworkInterface,
			Region:   region,
			ParentID: networkID,
			ID:       id,
		},
		Tags: labels,
		Attr: map[string]any{
			"name":      name,
			"networkId": networkID,
			"device":    device,
			"ipv4":      ipv4,
			"state":     status,
		},
	}

	return &meta, nil
}

func ResourceMetaFromPublicIP(publicIP iaas.PublicIp, projectId, region string) (*ResourceMeta, error) {
	labels, err := toStringMap(publicIP.GetLabels())
	if err != nil {
		return nil, err
	}

	id := publicIP.GetId()
	ip := publicIP.GetIp()
	networkInterfaceID := publicIP.GetNetworkInterface()

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    publicIPARN(region, projectId, id),
			Type:   ResourcePublicIP,
			Region: region,
			ID:     id,
		},
		Tags: labels,
		Attr: map[string]any{
			"name":             ip,
			"ip":               ip,
			"networkInterface": networkInterfaceID,
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
	expires := key.GetExpires()
	credentialsGroupID, _ := firstStringAdditionalProperty(key.AdditionalProperties, "credentialsGroupId", "credentialsGroupID", "groupId")

	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:      credentialARN(region, projectId, id),
			Type:     ResourceObjectStorageCredential,
			Region:   region,
			ParentID: credentialsGroupID,
			ID:       id,
		},
		Tags: map[string]string{},
		Attr: map[string]any{
			"name":    name,
			"expires": expires,
		},
	}

	for _, keyName := range []string{"credentialsGroupId", "credentialsGroupID", "credentialsGroupName", "groupId", "groupName", "userUrn", "userURN", "urn"} {
		if value, ok := stringAdditionalProperty(key.AdditionalProperties, keyName); ok {
			meta.Attr[keyName] = value
		}
	}

	return &meta, nil
}

func ResourceMetaFromCredentialsGroup(
	cg objectstorage.CredentialsGroup,
	projectId,
	region string,
) (*ResourceMeta, error) {

	id := cg.GetCredentialsGroupId()
	name := cg.GetDisplayName()
	urn := cg.GetUrn()

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
			"urn":  urn,
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

	if name, ok := meta.Attr["name"].(string); ok && (strings.HasPrefix(name, *deploymentId+"-") || name == *deploymentId) {
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

func stringAdditionalProperty(properties map[string]interface{}, key string) (string, bool) {
	if properties == nil {
		return "", false
	}

	value, ok := properties[key]
	if !ok {
		return "", false
	}

	stringValue, ok := value.(string)
	if !ok || stringValue == "" {
		return "", false
	}

	return stringValue, true
}

func firstStringAdditionalProperty(properties map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := stringAdditionalProperty(properties, key); ok {
			return value, true
		}
	}

	return "", false
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

func publicIPARN(region, projectId, id string) string {
	return fmt.Sprintf("stackit:%s:project:%s:public-ip:%s", region, projectId, id)
}

func nicARN(region, projectId, networkID, id string) string {
	return fmt.Sprintf("stackit:%s:project:%s:network:%s:nic:%s", region, projectId, networkID, id)
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
