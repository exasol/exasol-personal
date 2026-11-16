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
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	v3 "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"

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

// createExoscaleClient creates an Exoscale client using environment credentials
func createExoscaleClient(ctx context.Context, zone string) (*v3.Client, error) {
	apiKey := os.Getenv("EXOSCALE_API_KEY")
	apiSecret := os.Getenv("EXOSCALE_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("EXOSCALE_API_KEY and EXOSCALE_API_SECRET environment variables are required")
	}

	client, err := v3.NewClient(credentials.NewStaticCredentials(apiKey, apiSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to create Exoscale client: %w", err)
	}

	endpoint, err := client.GetZoneAPIEndpoint(ctx, v3.ZoneName(zone))
	if err != nil {
		return nil, fmt.Errorf("unsupported zone %s: %w", zone, err)
	}

	return client.WithEndpoint(endpoint), nil
}

// listZoneNames returns the names of all zones available to the organization.
func listZoneNames(ctx context.Context, client *v3.Client) ([]string, error) {
	resp, err := client.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	names := make([]string, 0, len(resp.Zones))
	for _, zone := range resp.Zones {
		names = append(names, string(zone.Name))
	}

	return names, nil
}

// detailsAccumulator carries the mutable state that the per-resource-type
// discovery phases of CollectDeploymentDetails contribute to.
type detailsAccumulator struct {
	details      *DeploymentDetails
	earliest     *time.Time
	hasInstances bool
	hasActive    bool
	hasStopped   bool
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

	acc := &detailsAccumulator{
		details: &DeploymentDetails{
			Summary: DeploymentSummary{
				ID:        deploymentID,
				Provider:  "exoscale",
				Region:    zone,
				Owner:     "",
				CreatedAt: time.Time{},
				State:     StateUnknown,
			},
		},
	}

	collectInstanceDetails(ctx, client, zone, deploymentID, acc)
	collectBlockStorageVolumeDetails(ctx, client, zone, deploymentID, acc)
	collectPrivateNetworkDetails(ctx, client, zone, deploymentID, acc)
	collectVPCDetails(ctx, client, zone, deploymentID, acc)
	collectSecurityGroupDetails(ctx, client, zone, deploymentID, acc)
	collectSSHKeyDetails(ctx, client, zone, deploymentID, acc)
	collectIAMRoleDetails(ctx, client, zone, deploymentID, acc)
	collectIAMAPIKeyDetails(ctx, client, zone, deploymentID, acc)
	collectSOSBucketDetails(ctx, zone, deploymentID, acc)

	applyDetailsSummaryTotals(acc)
	deriveDetailsSummaryState(acc)
	applyOwnerFallback(&acc.details.Summary)

	return acc.details, nil
}

// collectInstanceDetails discovers compute instances by label
func collectInstanceDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	instancesResp, err := client.ListInstances(ctx)
	if err != nil {
		slog.Debug("list instances failed", "error", err)
		return
	}

	if instancesResp == nil || instancesResp.Instances == nil {
		return
	}

	for _, inst := range instancesResp.Instances {
		collectInstanceDetail(ctx, client, zone, deploymentID, inst, acc)
	}
}

// collectInstanceDetail records a single compute instance if it belongs to the deployment
func collectInstanceDetail(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	inst v3.ListInstancesResponseInstances,
	acc *detailsAccumulator,
) {
	// inst.ID is v3.UUID which is a string type
	if inst.ID == "" {
		return
	}
	instID := inst.ID

	// Get full instance to access all fields
	fullInst, err := client.GetInstance(ctx, instID)
	if err != nil {
		slog.Debug("failed to get instance details", "error", err)
		return
	}

	nameStr := fullInst.Name
	labels := fullInst.Labels
	slog.Info("discovered instance", "id", instID, "name", nameStr, "labels", labels, "deploymentID", deploymentID)

	if !matchesDeployment(&nameStr, labels, deploymentID) {
		return
	}

	acc.hasInstances = true
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
		acc.earliest = preferEarlier(acc.earliest, &fullInst.CreatedAT)
	}

	if owner, ok := labels["owner"]; ok && owner != "" {
		acc.details.Summary.Owner = owner
	}

	switch state {
	case StateActive:
		acc.hasActive = true
	case StateStopped:
		acc.hasStopped = true
	}

	acc.details.Resources = append(acc.details.Resources, meta)
}

// collectBlockStorageVolumeDetails discovers block storage volumes by label
func collectBlockStorageVolumeDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	volumesResp, err := client.ListBlockStorageVolumes(ctx)
	if err != nil {
		slog.Debug("list block storage volumes failed", "error", err)
		return
	}

	if volumesResp == nil || volumesResp.BlockStorageVolumes == nil {
		return
	}

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
			acc.earliest = preferEarlier(acc.earliest, &vol.CreatedAT)
		}

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// collectPrivateNetworkDetails discovers private networks by label
func collectPrivateNetworkDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	networksResp, err := client.ListPrivateNetworks(ctx)
	if err != nil {
		slog.Debug("list private networks failed", "error", err)
		return
	}

	if networksResp == nil || networksResp.PrivateNetworks == nil {
		return
	}

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

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// collectSecurityGroupDetails discovers security groups by name pattern
func collectSecurityGroupDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	securityGroupsResp, err := client.ListSecurityGroups(ctx)
	if err != nil {
		slog.Debug("list security groups failed", "error", err)
		return
	}

	if securityGroupsResp == nil || securityGroupsResp.SecurityGroups == nil {
		return
	}

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

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// collectSSHKeyDetails discovers SSH keys by name pattern
func collectSSHKeyDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	sshKeysResp, err := client.ListSSHKeys(ctx)
	if err != nil {
		slog.Debug("list ssh keys failed", "error", err)
		return
	}

	if sshKeysResp == nil || sshKeysResp.SSHKeys == nil {
		return
	}

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

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// collectIAMRoleDetails discovers IAM roles by label
func collectIAMRoleDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	iamRolesResp, err := client.ListIAMRoles(ctx)
	if err != nil {
		slog.Debug("list iam roles failed", "error", err)
		return
	}

	if iamRolesResp == nil || iamRolesResp.IAMRoles == nil {
		return
	}

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

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// collectIAMAPIKeyDetails discovers IAM API keys by name pattern
func collectIAMAPIKeyDetails(
	ctx context.Context,
	client *v3.Client,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	apiKeysResp, err := client.ListAPIKeys(ctx)
	if err != nil {
		slog.Debug("list iam api keys failed", "error", err)
		return
	}

	if apiKeysResp == nil || apiKeysResp.APIKeys == nil {
		return
	}

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

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// collectSOSBucketDetails discovers SOS buckets by name pattern
func collectSOSBucketDetails(
	ctx context.Context,
	zone string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	sosBuckets, err := listSOSBuckets(ctx, zone, deploymentID)
	if err != nil {
		slog.Debug("list sos buckets failed", "error", err)
		return
	}

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

		acc.details.Resources = append(acc.details.Resources, meta)
	}
}

// applyDetailsSummaryTotals updates the summary resource count and creation time
func applyDetailsSummaryTotals(acc *detailsAccumulator) {
	acc.details.Summary.Resources = len(acc.details.Resources)
	if acc.earliest != nil {
		acc.details.Summary.CreatedAt = *acc.earliest
	}
}

// deriveDetailsSummaryState determines the deployment state from the discovered resources
func deriveDetailsSummaryState(acc *detailsAccumulator) {
	if acc.hasInstances {
		switch {
		case acc.hasActive:
			acc.details.Summary.State = StateActive
		case acc.hasStopped:
			acc.details.Summary.State = StateStopped
		case acc.details.Summary.Resources > 0:
			acc.details.Summary.State = StateTerminated
		}
	} else if acc.details.Summary.Resources > 0 {
		acc.details.Summary.State = "orphaned"
	}
}

// applyOwnerFallback substitutes a placeholder for an unknown owner
func applyOwnerFallback(summary *DeploymentSummary) {
	if summary.Owner == "" {
		summary.Owner = "-"
	}
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

	if err := discoverSummariesFromInstances(ctx, client, zone, ownerFilter, summaries); err != nil {
		return nil, err
	}
	discoverSummariesFromVolumes(ctx, client, zone, ownerFilter, summaries)
	discoverSummariesFromPrivateNetworks(ctx, client, zone, ownerFilter, summaries)
	discoverSummariesFromVPCs(ctx, client, zone, ownerFilter, summaries)
	discoverSummariesFromSOSBuckets(ctx, zone, ownerFilter, summaries)

	// Organization-wide resources come last, once every zonal resource has
	// established its deployment in this zone.
	addOrganizationResourcesToSummaries(
		ctx,
		listOrganizationResources(ctx, client),
		zone,
		ownerFilter,
		summaries,
		deploymentsInZonesOtherThan(client, zone),
	)

	return summariesToSlice(summaries), nil
}

// getOrCreateSummary returns the existing deployment summary entry or registers a new one
func getOrCreateSummary(
	summaries map[string]*DeploymentSummary,
	zone string,
	depID string,
	owner string,
	createdAt time.Time,
) *DeploymentSummary {
	if sum, ok := summaries[depID]; ok {
		return sum
	}
	sum := &DeploymentSummary{
		ID:        depID,
		Provider:  "exoscale",
		Region:    zone,
		Owner:     owner,
		CreatedAt: createdAt,
		State:     StateUnknown,
	}
	summaries[depID] = sum
	return sum
}

// discoverSummariesFromInstances discovers deployments from compute instances
func discoverSummariesFromInstances(
	ctx context.Context,
	client *v3.Client,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) error {
	instancesResp, err := client.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	if instancesResp == nil || instancesResp.Instances == nil {
		return nil
	}

	for _, inst := range instancesResp.Instances {
		depID, ok := inst.Labels["deployment_id"]
		if !ok || depID == "" {
			continue
		}

		owner := inst.Labels["owner"]
		if !ownerMatchesFilter(owner, ownerFilter) {
			continue
		}

		sum := getOrCreateSummary(summaries, zone, depID, owner, inst.CreatedAT)
		sum.Resources++

		applyEarlierCreatedAt(sum, inst.CreatedAT)
		applyInstanceStateToSummary(sum, inst.State)
	}

	return nil
}

// applyEarlierCreatedAt keeps the earliest known creation time on the summary
func applyEarlierCreatedAt(sum *DeploymentSummary, createdAt time.Time) {
	if !createdAt.IsZero() && (sum.CreatedAt.IsZero() || createdAt.Before(sum.CreatedAt)) {
		sum.CreatedAt = createdAt
	}
}

// applyInstanceStateToSummary promotes the summary state based on an instance state
func applyInstanceStateToSummary(sum *DeploymentSummary, instanceState v3.InstanceState) {
	stateStr := string(instanceState)
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

// discoverSummariesFromVolumes discovers deployments from block storage volumes
func discoverSummariesFromVolumes(
	ctx context.Context,
	client *v3.Client,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	volumesResp, _ := client.ListBlockStorageVolumes(ctx)
	if volumesResp == nil || volumesResp.BlockStorageVolumes == nil {
		return
	}

	for _, vol := range volumesResp.BlockStorageVolumes {
		depID, ok := vol.Labels["deployment_id"]
		if !ok || depID == "" {
			continue
		}

		owner := vol.Labels["owner"]
		if !ownerMatchesFilter(owner, ownerFilter) {
			continue
		}

		sum := getOrCreateSummary(summaries, zone, depID, owner, vol.CreatedAT)
		sum.Resources++

		applyEarlierCreatedAt(sum, vol.CreatedAT)
	}
}

// discoverSummariesFromPrivateNetworks discovers deployments from private networks
func discoverSummariesFromPrivateNetworks(
	ctx context.Context,
	client *v3.Client,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	networksResp, _ := client.ListPrivateNetworks(ctx)
	if networksResp == nil || networksResp.PrivateNetworks == nil {
		return
	}

	for _, net := range networksResp.PrivateNetworks {
		depID, ok := net.Labels["deployment_id"]
		if !ok || depID == "" {
			continue
		}

		owner := net.Labels["owner"]
		if !ownerMatchesFilter(owner, ownerFilter) {
			continue
		}

		sum := getOrCreateSummary(summaries, zone, depID, owner, time.Time{})
		sum.Resources++
	}
}

// discoverSummariesFromSOSBuckets discovers deployments from SOS bucket names.
// A failed apply can leave buckets behind without any labeled resource, so
// buckets must be able to establish a deployment on their own.
func discoverSummariesFromSOSBuckets(
	ctx context.Context,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	buckets, err := listDeploymentSOSBuckets(ctx, zone)
	if err != nil {
		slog.Warn("SOS bucket discovery failed", "zone", zone, "error", err)
		return
	}

	addSOSBucketsToSummaries(buckets, zone, ownerFilter, summaries)
}

// addSOSBucketsToSummaries counts the deployment buckets located in zone.
func addSOSBucketsToSummaries(
	buckets []s3types.Bucket,
	zone string,
	ownerFilter string,
	summaries map[string]*DeploymentSummary,
) {
	addNamedResourcesToSummaries(sosBucketsInZone(buckets, zone), zone, ownerFilter, summaries)
}

func bucketInZone(bucket s3types.Bucket, zone string) bool {
	return ptrString(bucket.BucketRegion) == zone
}

// deploymentIDFromResourceName returns the deployment id that prefixes a
// resource name such as exasol-1a2b3c4d-sos-archive, or "" if there is none.
func deploymentIDFromResourceName(name string) string {
	match := resourceNamePattern.FindStringSubmatch(name)
	if match == nil {
		return ""
	}

	return match[1]
}

// summariesToSlice converts the discovered deployment map into the returned slice
func summariesToSlice(summaries map[string]*DeploymentSummary) []DeploymentSummary {
	result := make([]DeploymentSummary, 0, len(summaries))
	for _, s := range summaries {
		applyOwnerFallback(s)
		result = append(result, *s)
	}

	return result
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

func preferEarlier(existing, candidate *time.Time) *time.Time {
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

// newSOSS3Client builds an S3 client for Exoscale's SOS API, pinning its
// credentials explicitly: CI environments commonly carry an unrelated AWS
// role's credentials, which Exoscale rejects if the SDK's default credential
// chain picks them up instead.
func newSOSS3Client(ctx context.Context, zone string) (*s3.Client, error) {
	apiKey := os.Getenv("EXOSCALE_API_KEY")
	apiSecret := os.Getenv("EXOSCALE_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("EXOSCALE_API_KEY and EXOSCALE_API_SECRET environment variables are required")
	}

	sosEndpoint := fmt.Sprintf("https://sos-%s.exo.io", zone)

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(zone),
		awsconfig.WithCredentialsProvider(awscreds.NewStaticCredentialsProvider(apiKey, apiSecret, "")),
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

	return s3.NewFromConfig(cfg), nil
}

// listSOSBuckets lists the SOS buckets belonging to a deployment
func listSOSBuckets(ctx context.Context, zone, deploymentID string) ([]string, error) {
	all, err := listDeploymentSOSBuckets(ctx, zone)
	if err != nil {
		return nil, err
	}

	var buckets []string
	for _, bucket := range all {
		if deploymentIDFromResourceName(ptrString(bucket.Name)) == deploymentID && bucketInZone(bucket, zone) {
			buckets = append(buckets, *bucket.Name)
		}
	}

	return buckets, nil
}

// listDeploymentSOSBuckets lists the buckets named after a deployment with
// their zone. SOS lists the buckets of every zone without one.
func listDeploymentSOSBuckets(ctx context.Context, zone string) ([]s3types.Bucket, error) {
	if os.Getenv("EXOSCALE_API_KEY") == "" {
		slog.Debug("SOS bucket discovery skipped: no credentials")
		return nil, nil
	}

	s3Client, err := newSOSS3Client(ctx, zone)
	if err != nil {
		return nil, err
	}

	var buckets []s3types.Bucket
	paginator := s3.NewListBucketsPaginator(s3Client, &s3.ListBucketsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list SOS buckets: %w", err)
		}
		for _, bucket := range page.Buckets {
			if deploymentIDFromResourceName(ptrString(bucket.Name)) == "" {
				continue
			}
			if resolveSOSBucketZone(ctx, s3Client, &bucket) {
				buckets = append(buckets, bucket)
			}
		}
	}

	return buckets, nil
}

// resolveSOSBucketZone sets the zone of a bucket listed without one and
// reports whether the zone is known.
func resolveSOSBucketZone(ctx context.Context, s3Client *s3.Client, bucket *s3types.Bucket) bool {
	if ptrString(bucket.BucketRegion) != "" {
		return true
	}

	location, err := s3Client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: bucket.Name})
	if err != nil {
		slog.Warn("skipping SOS bucket of unknown zone", "bucket", ptrString(bucket.Name), "error", err)
		return false
	}
	bucket.BucketRegion = awssdk.String(string(location.LocationConstraint))

	return true
}
