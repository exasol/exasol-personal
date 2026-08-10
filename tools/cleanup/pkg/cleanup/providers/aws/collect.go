// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	iam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	rgt "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

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

// UpdateCreatedAtFromTag prefers the CreatedAt tag value (RFC3339) if present.
// It returns the earlier of current and tag-provided timestamp. If current is zero, tag wins.
func UpdateCreatedAtFromTag(current time.Time, createdTag string) time.Time {
	if createdTag == "" {
		return current
	}
	if ts, err := time.Parse(time.RFC3339, createdTag); err == nil {
		if current.IsZero() || ts.Before(current) {
			return ts
		}
	}

	return current
}

// PreferEarlier returns candidate if it's earlier than base (or base is zero).
func PreferEarlier(base time.Time, candidate *time.Time) time.Time {
	if candidate == nil {
		return base
	}
	if base.IsZero() || candidate.Before(base) {
		return *candidate
	}

	return base
}

// detailsAccumulator carries the in-progress DeploymentDetails plus the
// cross-phase state (earliest timestamp, EC2 presence/state flags) that
// CollectDeploymentDetails' phases need to share.
type detailsAccumulator struct {
	details    *DeploymentDetails
	earliest   *time.Time
	hasEC2     bool
	hasActive  bool
	hasStopped bool
}

// CollectDeploymentDetails enumerates resources for a single deployment
// and enriches attributes and summary.
func CollectDeploymentDetails(
	ctx context.Context,
	region string,
	deploymentID string,
) (*DeploymentDetails, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := rgt.NewFromConfig(cfg)
	ec2Client := ec2.NewFromConfig(cfg)
	ssmClient := ssm.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)

	acc := &detailsAccumulator{
		details: &DeploymentDetails{
			Summary: DeploymentSummary{
				ID:        deploymentID,
				Provider:  "aws",
				Region:    region,
				Owner:     "",
				CreatedAt: time.Time{},
				State:     StateUnknown,
			},
		},
	}

	if err := discoverTaggedResources(ctx, client, region, deploymentID, acc); err != nil {
		return nil, err
	}
	acc.details.Summary.Resources = len(acc.details.Resources)

	// IAM discovery: roles and instance profiles are global and not returned by
	// Resource Groups Tagging API in regional queries. Discover via IAM API and tags.
	// Filter for Deployment=<deploymentID> tag on roles and instance profiles.
	discoverIAMRoles(ctx, iamClient, region, deploymentID, acc)
	discoverIAMInstanceProfiles(ctx, iamClient, region, deploymentID, acc)

	// Update resource count after IAM discovery
	acc.details.Summary.Resources = len(acc.details.Resources)

	enrichResourcesByType(ctx, ec2Client, ssmClient, s3Client, acc)

	// Apply earliest resource time only if it's earlier than tag-based CreatedAt
	acc.details.Summary.CreatedAt = PreferEarlier(acc.details.Summary.CreatedAt, acc.earliest)
	if acc.details.Summary.Owner == "" {
		acc.details.Summary.Owner = "-"
	}
	deriveDetailsState(acc)

	return acc.details, nil
}

// discoverTaggedResources paginates the Resource Groups Tagging API for all
// resources tagged with Deployment=<deploymentID>, recording each as a
// ResourceMeta and deriving the summary owner/createdAt from tags.
func discoverTaggedResources(
	ctx context.Context,
	client *rgt.Client,
	region string,
	deploymentID string,
	acc *detailsAccumulator,
) error {
	paginationToken := ""
	for {
		input := &rgt.GetResourcesInput{
			TagFilters: []rgttypes.TagFilter{
				{Key: aws.String("Deployment"), Values: []string{deploymentID}},
			},
			ResourcesPerPage: aws.Int32(resourcesPerPage),
		}
		if paginationToken != "" {
			input.PaginationToken = aws.String(paginationToken)
		}
		out, err := client.GetResources(ctx, input)
		if err != nil {
			return err
		}
		for _, mapping := range out.ResourceTagMappingList {
			recordTaggedResource(mapping, region, acc)
		}
		if out.PaginationToken == nil || *out.PaginationToken == "" {
			break
		}
		paginationToken = *out.PaginationToken
	}

	return nil
}

// recordTaggedResource records a single tagged-resource mapping as a
// ResourceMeta and folds its Owner/CreatedAt tags into the summary.
func recordTaggedResource(mapping rgttypes.ResourceTagMapping, region string, acc *detailsAccumulator) {
	arn := awsString(mapping.ResourceARN)
	rType, rID := classifyARN(arn)
	// Do NOT skip unclassified: include in output
	// so resource counts match discover and users can see raw entries
	meta := ResourceMeta{
		Ref:  ResourceRef{ARN: arn, Type: rType, Region: region, ID: rID},
		Tags: tagsToMap(mapping.Tags),
		Attr: map[string]any{},
	}
	acc.details.Resources = append(acc.details.Resources, meta)
	if acc.details.Summary.Owner == "" {
		if o := meta.Tags["Owner"]; o != "" {
			acc.details.Summary.Owner = o
		}
	}
	// CreatedAt tag wins first
	acc.details.Summary.CreatedAt = UpdateCreatedAtFromTag(
		acc.details.Summary.CreatedAt,
		meta.Tags["CreatedAt"],
	)
}

// discoverIAMRoles finds IAM roles matching deploymentID by tag or name.
// IAM is global and not covered by the regional Resource Groups Tagging API query.
func discoverIAMRoles(
	ctx context.Context,
	iamClient *iam.Client,
	region string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	slog.Debug("Starting IAM discovery", "deploymentID", deploymentID)
	var marker *string
	rolesChecked := 0
	for {
		out, err := iamClient.ListRoles(ctx, &iam.ListRolesInput{Marker: marker})
		if err != nil {
			slog.Debug("IAM list roles failed", "error", err)
			break
		}
		rolesChecked += len(out.Roles)
		for _, role := range out.Roles {
			recordIAMRoleIfMatching(ctx, iamClient, region, deploymentID, role, acc)
		}
		if out.IsTruncated && out.Marker != nil {
			marker = out.Marker
		} else {
			break
		}
	}
	slog.Debug("IAM role discovery complete", "rolesChecked", rolesChecked)
}

// recordIAMRoleIfMatching fetches a role's tags and records it as a
// ResourceMeta if it's tagged or named for the deployment.
func recordIAMRoleIfMatching(
	ctx context.Context,
	iamClient *iam.Client,
	region string,
	deploymentID string,
	role iamtypes.Role,
	acc *detailsAccumulator,
) {
	// Fetch role tags and check Deployment match
	tagsOut, tErr := iamClient.ListRoleTags(ctx, &iam.ListRoleTagsInput{RoleName: role.RoleName})
	if tErr != nil {
		return
	}
	hasTag := hasIAMDeploymentTag(tagsOut.Tags, deploymentID)
	hasName := role.RoleName != nil && strings.Contains(*role.RoleName, deploymentID)
	if !hasTag && !hasName {
		return
	}
	slog.Debug("IAM role matched", "name", awsString(role.RoleName), "hasTag", hasTag, "hasName", hasName)
	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    awsString(role.Arn),
			Type:   ResourceIAMRole,
			Region: region,
			ID:     awsString(role.RoleName),
		},
		Tags: iamTagsToMap(tagsOut.Tags),
		Attr: map[string]any{},
	}
	if role.CreateDate != nil {
		meta.Attr["createTime"] = *role.CreateDate
	}
	acc.details.Resources = append(acc.details.Resources, meta)
}

// discoverIAMInstanceProfiles finds IAM instance profiles matching
// deploymentID by tag or name, mirroring discoverIAMRoles.
func discoverIAMInstanceProfiles(
	ctx context.Context,
	iamClient *iam.Client,
	region string,
	deploymentID string,
	acc *detailsAccumulator,
) {
	var marker *string
	for {
		out, err := iamClient.ListInstanceProfiles(ctx, &iam.ListInstanceProfilesInput{Marker: marker})
		if err != nil {
			slog.Debug("IAM list instance profiles failed", "error", err)
			break
		}
		for _, prof := range out.InstanceProfiles {
			recordIAMInstanceProfileIfMatching(ctx, iamClient, region, deploymentID, prof, acc)
		}
		if out.IsTruncated && out.Marker != nil {
			marker = out.Marker
		} else {
			break
		}
	}
}

// recordIAMInstanceProfileIfMatching fetches an instance profile's tags and
// records it as a ResourceMeta if it's tagged or named for the deployment.
func recordIAMInstanceProfileIfMatching(
	ctx context.Context,
	iamClient *iam.Client,
	region string,
	deploymentID string,
	prof iamtypes.InstanceProfile,
	acc *detailsAccumulator,
) {
	tagsOut, tErr := iamClient.ListInstanceProfileTags(ctx, &iam.ListInstanceProfileTagsInput{InstanceProfileName: prof.InstanceProfileName})
	if tErr != nil {
		return
	}
	if !hasIAMDeploymentTag(tagsOut.Tags, deploymentID) &&
		!(prof.InstanceProfileName != nil && strings.Contains(*prof.InstanceProfileName, deploymentID)) {
		return
	}
	meta := ResourceMeta{
		Ref: ResourceRef{
			ARN:    awsString(prof.Arn),
			Type:   ResourceIAMInstProf,
			Region: region,
			ID:     awsString(prof.InstanceProfileName),
		},
		Tags: iamTagsToMap(tagsOut.Tags),
		Attr: map[string]any{},
	}
	if prof.CreateDate != nil {
		meta.Attr["createTime"] = *prof.CreateDate
	}
	acc.details.Resources = append(acc.details.Resources, meta)
}

// enrichResourcesByType dispatches each discovered resource to its
// type-specific enrichment, tracking cross-resource state in acc.
func enrichResourcesByType(
	ctx context.Context,
	ec2Client *ec2.Client,
	ssmClient *ssm.Client,
	s3Client *s3.Client,
	acc *detailsAccumulator,
) {
	for i := range acc.details.Resources {
		meta := &acc.details.Resources[i]
		switch meta.Ref.Type {
		case ResourceIAMRole, ResourceIAMInstProf:
			enrichIAMResource(meta, acc)
		case ResourceEC2Instance:
			enrichEC2Instance(ctx, ec2Client, meta, acc)
		case ResourceEBSVolume:
			enrichEBSVolume(ctx, ec2Client, meta, acc)
		case ResourceSSMParam:
			enrichSSMParam(ctx, ssmClient, meta, acc)
		case ResourceS3Bucket:
			enrichS3Bucket(ctx, s3Client, meta)
		case ResourceEC2KeyPair,
			ResourceInternetGW,
			ResourceRouteTable,
			ResourceSecurityGrp,
			ResourceSubnet,
			ResourceVPC:
		default:
			// no enrichment required here; handled by delete phase or not applicable
		}
	}
}

// enrichIAMResource marks presence/state for an IAM role or instance profile
// and folds its already-recorded createTime into the earliest timestamp.
func enrichIAMResource(meta *ResourceMeta, acc *detailsAccumulator) {
	// Basic presence and created time already recorded; mark state
	if _, ok := meta.Attr["state"]; !ok {
		meta.Attr["state"] = "present"
	}
	if ct, ok := meta.Attr["createTime"].(time.Time); ok {
		acc.earliest = preferPtrEarlier(acc.earliest, &ct)
	}
}

// enrichEC2Instance describes an EC2 instance and records its launch time and state.
func enrichEC2Instance(
	ctx context.Context,
	ec2Client *ec2.Client,
	meta *ResourceMeta,
	acc *detailsAccumulator,
) {
	acc.hasEC2 = true
	out, err := ec2Client.DescribeInstances(
		ctx,
		&ec2.DescribeInstancesInput{InstanceIds: []string{meta.Ref.ID}},
	)
	if err != nil {
		slog.Debug("describe instance failed", "id", meta.Ref.ID, "error", err)
		if _, ok := meta.Attr["state"]; !ok {
			meta.Attr["state"] = StateUnknown
		}

		return
	}
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			if inst.LaunchTime != nil {
				meta.Attr["launchTime"] = *inst.LaunchTime
				acc.earliest = preferPtrEarlier(acc.earliest, inst.LaunchTime)
			}
			st := ec2StateToSimple(inst.State)
			meta.Attr["state"] = st
			switch st {
			case StateActive, StateProvisioning:
				acc.hasActive = true
			case StateStopped:
				acc.hasStopped = true
			default:
				// no-op
			}
		}
	}
}

// enrichEBSVolume describes an EBS volume and records its creation time and state.
func enrichEBSVolume(
	ctx context.Context,
	ec2Client *ec2.Client,
	meta *ResourceMeta,
	acc *detailsAccumulator,
) {
	out, err := ec2Client.DescribeVolumes(
		ctx,
		&ec2.DescribeVolumesInput{VolumeIds: []string{meta.Ref.ID}},
	)
	if err != nil {
		slog.Debug("describe volume failed", "id", meta.Ref.ID, "error", err)
		if _, ok := meta.Attr["state"]; !ok {
			meta.Attr["state"] = StateUnknown
		}

		return
	}
	for _, volume := range out.Volumes {
		if volume.CreateTime != nil {
			meta.Attr["createTime"] = *volume.CreateTime
			acc.earliest = preferPtrEarlier(acc.earliest, volume.CreateTime)
		}
		if volume.State != "" {
			meta.Attr["state"] = string(volume.State)
		}
	}
}

// enrichSSMParam describes an SSM parameter and records its last-modified time.
func enrichSSMParam(
	ctx context.Context,
	ssmClient *ssm.Client,
	meta *ResourceMeta,
	acc *detailsAccumulator,
) {
	out, err := ssmClient.DescribeParameters(
		ctx,
		&ssm.DescribeParametersInput{
			Filters: []ssmtypes.ParametersFilter{
				{Key: ssmtypes.ParametersFilterKeyName, Values: []string{meta.Ref.ID}},
			},
		},
	)
	if err != nil {
		slog.Debug("describe parameter failed", "name", meta.Ref.ID, "error", err)
		if _, ok := meta.Attr["state"]; !ok {
			meta.Attr["state"] = StateUnknown
		}

		return
	}
	for _, p := range out.Parameters {
		if p.LastModifiedDate != nil {
			meta.Attr["lastModified"] = *p.LastModifiedDate
			acc.earliest = preferPtrEarlier(acc.earliest, p.LastModifiedDate)
		}
	}
	if _, ok := meta.Attr["state"]; !ok {
		meta.Attr["state"] = "present"
	}
}

// enrichS3Bucket records an S3 bucket's versioning status.
func enrichS3Bucket(
	ctx context.Context,
	s3Client *s3.Client,
	meta *ResourceMeta,
) {
	vOut, err := s3Client.GetBucketVersioning(
		ctx,
		&s3.GetBucketVersioningInput{Bucket: aws.String(meta.Ref.ID)},
	)
	if err != nil {
		slog.Debug("get bucket versioning failed", "bucket", meta.Ref.ID, "error", err)
		if _, ok := meta.Attr["state"]; !ok {
			meta.Attr["state"] = StateUnknown
		}

		return
	}
	if vOut.Status != "" {
		meta.Attr["versioning"] = string(vOut.Status)
	}
	meta.Attr["state"] = "present"
}

// deriveDetailsState determines the deployment state from the discovered and enriched resources.
func deriveDetailsState(acc *detailsAccumulator) {
	if acc.hasEC2 {
		switch {
		case acc.hasActive:
			acc.details.Summary.State = StateActive
		case acc.hasStopped:
			acc.details.Summary.State = StateStopped
		case acc.details.Summary.Resources > 0:
			acc.details.Summary.State = StateTerminated
		default:
			// no-op
		}
	} else if acc.details.Summary.Resources > 0 {
		acc.details.Summary.State = "orphaned"
	}
}

// summariesAccumulator carries the in-progress deployment summaries plus the
// resource-ID buckets that CollectDeploymentSummaries' later enrichment
// phases need.
type summariesAccumulator struct {
	summaries   map[string]*DeploymentSummary
	instanceIDs map[string][]string
	volumeIDs   map[string][]string
	ssmNames    map[string][]string
	processed   map[string]struct{}
}

// CollectDeploymentSummaries discovers deployments across the account/region with filters,
// deriving CreatedAt and state using shared precedence rules.
func CollectDeploymentSummaries(
	ctx context.Context,
	region string,
	ownerFilter string,
	legacy bool,
) ([]DeploymentSummary, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := rgt.NewFromConfig(cfg)
	ec2Client := ec2.NewFromConfig(cfg)

	acc := &summariesAccumulator{
		summaries:   map[string]*DeploymentSummary{},
		instanceIDs: map[string][]string{},
		volumeIDs:   map[string][]string{},
		ssmNames:    map[string][]string{},
		processed:   map[string]struct{}{},
	}
	// compile strict deployment tag regex once
	deploymentIDRegex := regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)

	var tagFilters []rgttypes.TagFilter
	if legacy {
		// Legacy mode: Only discover via Deployment tag presence (ignore Project tag)
		tagFilters = []rgttypes.TagFilter{{Key: aws.String("Deployment")}}
	} else {
		// Default: Require Project tag to be exasol-personal
		tagFilters = []rgttypes.TagFilter{{Key: aws.String("Project"), Values: []string{"exasol-personal"}}}
	}
	if err := discoverTaggedSummaries(ctx, client, region, tagFilters, deploymentIDRegex, ownerFilter, acc); err != nil {
		return nil, err
	}

	enrichSummariesFromEC2(ctx, ec2Client, acc)
	applyEBSFallbackCreatedAt(ctx, ec2Client, acc)
	applySSMFallbackCreatedAt(ctx, cfg, acc)
	markOrphanedSummaries(acc)

	return summariesSliceFromMap(acc.summaries), nil
}

// discoverTaggedSummaries paginates the Resource Groups Tagging API for the
// given tag filters, registering/updating a DeploymentSummary per matching
// deployment and bucketing resource IDs by type for later enrichment.
func discoverTaggedSummaries(
	ctx context.Context,
	client *rgt.Client,
	region string,
	tagFilters []rgttypes.TagFilter,
	deploymentIDRegex *regexp.Regexp,
	ownerFilter string,
	acc *summariesAccumulator,
) error {
	paginationToken := ""
	for {
		input := &rgt.GetResourcesInput{
			TagFilters:       tagFilters,
			ResourcesPerPage: aws.Int32(resourcesPerPage),
		}
		if paginationToken != "" {
			input.PaginationToken = aws.String(paginationToken)
		}
		out, err := client.GetResources(ctx, input)
		if err != nil {
			return err
		}
		for _, mapping := range out.ResourceTagMappingList {
			recordTaggedSummaryResource(mapping, region, deploymentIDRegex, ownerFilter, acc)
		}
		if out.PaginationToken == nil || *out.PaginationToken == "" {
			break
		}
		paginationToken = *out.PaginationToken
	}

	return nil
}

// recordTaggedSummaryResource filters a single tagged resource mapping by
// deployment-ID format and owner, dedupes it by ARN, updates or creates the
// deployment's summary, and buckets the resource ID by type.
func recordTaggedSummaryResource(
	mapping rgttypes.ResourceTagMapping,
	region string,
	deploymentIDRegex *regexp.Regexp,
	ownerFilter string,
	acc *summariesAccumulator,
) {
	deploymentID := tagValue(mapping.Tags, "Deployment")
	// Applies to both modern and legacy deployments: ensure the
	// deployment tag matches the strict format.
	if !deploymentIDRegex.MatchString(deploymentID) {
		return
	}

	ownerTag := tagValue(mapping.Tags, "Owner")
	if !ownerMatchesFilter(ownerTag, ownerFilter) {
		return
	}
	arn := awsString(mapping.ResourceARN)
	if _, seen := acc.processed[arn]; seen {
		return
	}
	acc.processed[arn] = struct{}{}
	reg := parseRegionFromARN(arn)
	if reg == "" {
		reg = region
	}
	sum := acc.summaries[deploymentID]
	if sum == nil {
		sum = &DeploymentSummary{
			ID:        deploymentID,
			Provider:  "aws",
			Region:    reg,
			Owner:     ownerTag,
			CreatedAt: time.Time{},
			State:     "unknown",
		}
		acc.summaries[deploymentID] = sum
	} else if sum.Owner == "" {
		sum.Owner = ownerTag
	}
	// CreatedAt tag wins first
	sum.CreatedAt = UpdateCreatedAtFromTag(
		sum.CreatedAt,
		tagValue(mapping.Tags, "CreatedAt"),
	)
	sum.Resources++
	rType, rID := classifyARN(arn)
	switch rType {
	case ResourceEC2Instance:
		if rID != "" {
			acc.instanceIDs[deploymentID] = append(acc.instanceIDs[deploymentID], rID)
		}
	case ResourceEBSVolume:
		if rID != "" {
			acc.volumeIDs[deploymentID] = append(acc.volumeIDs[deploymentID], rID)
		}
	case ResourceSSMParam:
		if rID != "" {
			acc.ssmNames[deploymentID] = append(acc.ssmNames[deploymentID], rID)
		}
	case ResourceEC2KeyPair,
		ResourceInternetGW,
		ResourceRouteTable,
		ResourceSecurityGrp,
		ResourceSubnet,
		ResourceVPC,
		ResourceS3Bucket:
	default:
		// ignore other resource types for summaries aggregation here
	}
}

// enrichSummariesFromEC2 describes every collected EC2 instance per
// deployment and derives the deployment's CreatedAt and lifecycle state.
func enrichSummariesFromEC2(ctx context.Context, ec2Client *ec2.Client, acc *summariesAccumulator) {
	for depID, ids := range acc.instanceIDs {
		enrichSummaryFromEC2Instances(ctx, ec2Client, depID, ids, acc)
	}
}

// enrichSummaryFromEC2Instances describes one deployment's EC2 instances and
// derives its CreatedAt and lifecycle state from the results.
func enrichSummaryFromEC2Instances(
	ctx context.Context,
	ec2Client *ec2.Client,
	depID string,
	ids []string,
	acc *summariesAccumulator,
) {
	out, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
	if err != nil {
		slog.Debug("instance describe failed", "deployment", depID, "error", err)
		return
	}
	var earliest *time.Time
	hasActive := false
	hasStopped := false
	foundAny := false
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			foundAny = true
			earliest = preferPtrEarlier(earliest, inst.LaunchTime)
			switch ec2StateToSimple(inst.State) {
			case StateActive, StateProvisioning:
				hasActive = true
			case StateStopped:
				hasStopped = true
			default:
				// no update
			}
		}
	}
	if earliest != nil {
		s := acc.summaries[depID]
		s.CreatedAt = PreferEarlier(s.CreatedAt, earliest)
	}
	switch {
	case hasActive:
		acc.summaries[depID].State = StateActive
	case hasStopped:
		acc.summaries[depID].State = StateStopped
	case !foundAny:
		acc.summaries[depID].State = StateTerminated
	default:
		// no update
	}
}

// applyEBSFallbackCreatedAt fills in a deployment's CreatedAt from its EBS
// volumes when no earlier source has already set it.
func applyEBSFallbackCreatedAt(ctx context.Context, ec2Client *ec2.Client, acc *summariesAccumulator) {
	for depID, ids := range acc.volumeIDs {
		sum := acc.summaries[depID]
		if sum == nil {
			continue
		}
		if !sum.CreatedAt.IsZero() {
			continue
		}
		out, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: ids})
		if err != nil {
			continue
		}
		var earliest *time.Time
		for _, v := range out.Volumes {
			earliest = preferPtrEarlier(earliest, v.CreateTime)
		}
		if earliest != nil {
			sum.CreatedAt = PreferEarlier(sum.CreatedAt, earliest)
		}
	}
}

// applySSMFallbackCreatedAt fills in a deployment's CreatedAt from its SSM
// parameters when no earlier source has already set it.
func applySSMFallbackCreatedAt(ctx context.Context, cfg aws.Config, acc *summariesAccumulator) {
	if len(acc.ssmNames) == 0 {
		return
	}
	ssmClient := ssm.NewFromConfig(cfg)
	for depID, names := range acc.ssmNames {
		sum := acc.summaries[depID]
		if sum == nil || !sum.CreatedAt.IsZero() {
			continue
		}
		out, err := ssmClient.DescribeParameters(
			ctx,
			&ssm.DescribeParametersInput{
				Filters: []ssmtypes.ParametersFilter{
					{Key: ssmtypes.ParametersFilterKeyName, Values: names},
				},
			},
		)
		if err != nil {
			continue
		}
		var earliest *time.Time
		for _, p := range out.Parameters {
			earliest = preferPtrEarlier(earliest, p.LastModifiedDate)
		}
		if earliest != nil {
			sum.CreatedAt = PreferEarlier(sum.CreatedAt, earliest)
		}
	}
}

// markOrphanedSummaries flags deployments with resources but no EC2
// instances (and otherwise-unknown state) as orphaned.
func markOrphanedSummaries(acc *summariesAccumulator) {
	for depID, sum := range acc.summaries {
		if sum.Resources > 0 {
			if _, ok := acc.instanceIDs[depID]; !ok {
				if sum.State == StateUnknown {
					sum.State = "orphaned"
				}
			}
		}
	}
}

// summariesSliceFromMap converts the discovered deployment map into a slice.
func summariesSliceFromMap(summaries map[string]*DeploymentSummary) []DeploymentSummary {
	result := make([]DeploymentSummary, 0, len(summaries))
	for _, s := range summaries {
		result = append(result, *s)
	}

	return result
}

// preferPtrEarlier returns the earlier non-nil pointer, else existing.
func preferPtrEarlier(existing, candidate *time.Time) *time.Time {
	if candidate == nil {
		return existing
	}
	if existing == nil || candidate.Before(*existing) {
		return candidate
	}

	return existing
}

// tagsToMap converts Tagging API tags to a simple map.
func tagsToMap(tags []rgttypes.Tag) map[string]string {
	mapped := make(map[string]string)
	for _, t := range tags {
		k := awsString(t.Key)
		v := awsString(t.Value)
		if k != "" {
			mapped[k] = v
		}
	}

	return mapped
}

// awsString dereferences a string pointer safely.
func awsString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}

// Helper implementations previously in discover/show, centralized here.
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

func parseRegionFromARN(arn string) string {
	if !strings.HasPrefix(arn, "arn:") {
		return ""
	}
	parts := strings.SplitN(arn, ":", arnSplitParts)
	if len(parts) < arnSplitParts {
		return ""
	}

	return parts[3]
}

func ec2StateToSimple(st *ec2types.InstanceState) string {
	if st == nil {
		return "unknown"
	}
	switch st.Name {
	case ec2types.InstanceStateNameRunning:
		return "active"
	case ec2types.InstanceStateNamePending:
		return "provisioning"
	case ec2types.InstanceStateNameStopped, ec2types.InstanceStateNameStopping:
		return "stopped"
	case ec2types.InstanceStateNameTerminated, ec2types.InstanceStateNameShuttingDown:
		return "terminated"
	default:
		return "unknown"
	}
}

func tagValue(tags []rgttypes.Tag, key string) string {
	for _, t := range tags {
		if awsString(t.Key) == key {
			return awsString(t.Value)
		}
	}

	return ""
}

// hasIAMDeploymentTag checks a slice of IAM tags for Deployment=deploymentID
func hasIAMDeploymentTag(tags []iamtypes.Tag, deploymentID string) bool {
	for _, t := range tags {
		if t.Key != nil && *t.Key == "Deployment" && t.Value != nil && *t.Value == deploymentID {
			return true
		}
	}
	return false
}

// iamTagsToMap converts IAM Tag structs to map[string]string
func iamTagsToMap(tags []iamtypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		var k, v string
		if t.Key != nil {
			k = *t.Key
		}
		if t.Value != nil {
			v = *t.Value
		}
		if k != "" {
			m[k] = v
		}
	}
	return m
}
