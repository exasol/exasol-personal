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

// CollectDeploymentDetails enumerates resources for a single deployment
// and enriches attributes and summary.
// function aggregates multiple AWS resources and states; split would harm cohesion.
// nolint: gocyclo, maintidx
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

	details := &DeploymentDetails{
		Summary: DeploymentSummary{
			ID:        deploymentID,
			Provider:  "aws",
			Region:    region,
			Owner:     "",
			CreatedAt: time.Time{},
			State:     StateUnknown,
		},
	}

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
			return nil, err
		}
		for _, mapping := range out.ResourceTagMappingList {
			arn := awsString(mapping.ResourceARN)
			rType, rID := classifyARN(arn)
			// Do NOT skip unclassified: include in output
			// so resource counts match discover and users can see raw entries
			meta := ResourceMeta{
				Ref:  ResourceRef{ARN: arn, Type: rType, Region: region, ID: rID},
				Tags: tagsToMap(mapping.Tags),
				Attr: map[string]any{},
			}
			details.Resources = append(details.Resources, meta)
			if details.Summary.Owner == "" {
				if o := meta.Tags["Owner"]; o != "" {
					details.Summary.Owner = o
				}
			}
			// CreatedAt tag wins first
			details.Summary.CreatedAt = UpdateCreatedAtFromTag(
				details.Summary.CreatedAt,
				meta.Tags["CreatedAt"],
			)
		}
		if out.PaginationToken == nil || *out.PaginationToken == "" {
			break
		}
		paginationToken = *out.PaginationToken
	}
	details.Summary.Resources = len(details.Resources)

	// IAM discovery: roles and instance profiles are global and not returned by
	// Resource Groups Tagging API in regional queries. Discover via IAM API and tags.
	// Filter for Deployment=<deploymentID> tag on roles and instance profiles.
	//nolint:nestif // acceptable nested flow for paging and tag checks
	{
		slog.Debug("Starting IAM discovery", "deploymentID", deploymentID)
		// List roles
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
				// Fetch role tags and check Deployment match
				tagsOut, tErr := iamClient.ListRoleTags(ctx, &iam.ListRoleTagsInput{RoleName: role.RoleName})
				if tErr == nil {
					hasTag := hasIAMDeploymentTag(tagsOut.Tags, deploymentID)
					hasName := role.RoleName != nil && strings.Contains(*role.RoleName, deploymentID)
					if hasTag || hasName {
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
						details.Resources = append(details.Resources, meta)
					}
				}
			}
			if out.IsTruncated && out.Marker != nil {
				marker = out.Marker
			} else {
				break
			}
		}
		slog.Debug("IAM role discovery complete", "rolesChecked", rolesChecked)

		// List instance profiles
		marker = nil
		for {
			out, err := iamClient.ListInstanceProfiles(ctx, &iam.ListInstanceProfilesInput{Marker: marker})
			if err != nil {
				slog.Debug("IAM list instance profiles failed", "error", err)
				break
			}
			for _, prof := range out.InstanceProfiles {
				tagsOut, tErr := iamClient.ListInstanceProfileTags(ctx, &iam.ListInstanceProfileTagsInput{InstanceProfileName: prof.InstanceProfileName})
				if tErr == nil {
					if hasIAMDeploymentTag(tagsOut.Tags, deploymentID) || (prof.InstanceProfileName != nil && strings.Contains(*prof.InstanceProfileName, deploymentID)) {
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
						details.Resources = append(details.Resources, meta)
					}
				}
			}
			if out.IsTruncated && out.Marker != nil {
				marker = out.Marker
			} else {
				break
			}
		}
	}

	// Update resource count after IAM discovery
	details.Summary.Resources = len(details.Resources)

	// Enrichment and state derivation
	var earliest *time.Time
	hasEC2 := false
	hasActive := false
	hasStopped := false
	for i := range details.Resources {
		meta := &details.Resources[i]
		switch meta.Ref.Type {
		case ResourceIAMRole:
			// Basic presence and created time already recorded; mark state
			if _, ok := meta.Attr["state"]; !ok {
				meta.Attr["state"] = "present"
			}
			if ct, ok := meta.Attr["createTime"].(time.Time); ok {
				earliest = preferPtrEarlier(earliest, &ct)
			}
		case ResourceIAMInstProf:
			if _, ok := meta.Attr["state"]; !ok {
				meta.Attr["state"] = "present"
			}
			if ct, ok := meta.Attr["createTime"].(time.Time); ok {
				earliest = preferPtrEarlier(earliest, &ct)
			}
		case ResourceEC2Instance:
			hasEC2 = true
			out, err := ec2Client.DescribeInstances(
				ctx,
				&ec2.DescribeInstancesInput{InstanceIds: []string{meta.Ref.ID}},
			)
			if err == nil {
				for _, res := range out.Reservations {
					for _, inst := range res.Instances {
						if inst.LaunchTime != nil {
							meta.Attr["launchTime"] = *inst.LaunchTime
							earliest = preferPtrEarlier(earliest, inst.LaunchTime)
						}
						st := ec2StateToSimple(inst.State)
						meta.Attr["state"] = st
						switch st {
						case StateActive, StateProvisioning:
							hasActive = true
						case StateStopped:
							hasStopped = true
						default:
							// no-op
						}
					}
				}
			} else {
				slog.Debug("describe instance failed", "id", meta.Ref.ID, "error", err)
				if _, ok := meta.Attr["state"]; !ok {
					meta.Attr["state"] = StateUnknown
				}
			}
		case ResourceEBSVolume:
			out, err := ec2Client.DescribeVolumes(
				ctx,
				&ec2.DescribeVolumesInput{VolumeIds: []string{meta.Ref.ID}},
			)
			if err == nil {
				for _, volume := range out.Volumes {
					if volume.CreateTime != nil {
						meta.Attr["createTime"] = *volume.CreateTime
						earliest = preferPtrEarlier(earliest, volume.CreateTime)
					}
					if volume.State != "" {
						meta.Attr["state"] = string(volume.State)
					}
				}
			} else {
				slog.Debug("describe volume failed", "id", meta.Ref.ID, "error", err)
				if _, ok := meta.Attr["state"]; !ok {
					meta.Attr["state"] = StateUnknown
				}
			}
		case ResourceSSMParam:
			out, err := ssmClient.DescribeParameters(
				ctx,
				&ssm.DescribeParametersInput{
					Filters: []ssmtypes.ParametersFilter{
						{Key: ssmtypes.ParametersFilterKeyName, Values: []string{meta.Ref.ID}},
					},
				},
			)
			if err == nil {
				for _, p := range out.Parameters {
					if p.LastModifiedDate != nil {
						meta.Attr["lastModified"] = *p.LastModifiedDate
						earliest = preferPtrEarlier(earliest, p.LastModifiedDate)
					}
				}
				if _, ok := meta.Attr["state"]; !ok {
					meta.Attr["state"] = "present"
				}
			} else {
				slog.Debug("describe parameter failed", "name", meta.Ref.ID, "error", err)
				if _, ok := meta.Attr["state"]; !ok {
					meta.Attr["state"] = StateUnknown
				}
			}
		case ResourceS3Bucket:
			vOut, err := s3Client.GetBucketVersioning(
				ctx,
				&s3.GetBucketVersioningInput{Bucket: aws.String(meta.Ref.ID)},
			)
			if err == nil {
				if vOut.Status != "" {
					meta.Attr["versioning"] = string(vOut.Status)
				}
				meta.Attr["state"] = "present"
			} else {
				slog.Debug("get bucket versioning failed", "bucket", meta.Ref.ID, "error", err)
				if _, ok := meta.Attr["state"]; !ok {
					meta.Attr["state"] = StateUnknown
				}
			}
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
	// Apply earliest resource time only if it's earlier than tag-based CreatedAt
	details.Summary.CreatedAt = PreferEarlier(details.Summary.CreatedAt, earliest)
	if details.Summary.Owner == "" {
		details.Summary.Owner = "-"
	}
	if hasEC2 {
		switch {
		case hasActive:
			details.Summary.State = StateActive
		case hasStopped:
			details.Summary.State = StateStopped
		case details.Summary.Resources > 0:
			details.Summary.State = StateTerminated
		default:
			// no-op
		}
	} else if details.Summary.Resources > 0 {
		details.Summary.State = "orphaned"
	}

	return details, nil
}

// CollectDeploymentSummaries discovers deployments across the account/region with filters,
// deriving CreatedAt and state using shared precedence rules.
// function coordinates cross-service summarization; refactor would reduce clarity.
// nolint: gocyclo, maintidx
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

	summaries := map[string]*DeploymentSummary{}
	instanceIDs := map[string][]string{}
	volumeIDs := map[string][]string{}
	ssmNames := map[string][]string{}
	processed := map[string]struct{}{}

	// Helper: compile strict deployment tag regex once
	deploymentIDRegex := regexp.MustCompile(`^exasol-[a-f0-9]{8}$`)

	// Helper to process resources for given tag filters
	processWithFilters := func(tagFilters []rgttypes.TagFilter) error {
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
				// Primary indicator: Project == exasol-personal
				projectTag := tagValue(mapping.Tags, "Project")
				deploymentID := tagValue(mapping.Tags, "Deployment")
				isModern := projectTag == "exasol-personal"
				// Legacy deployments: no Project tag
				if !isModern {
					if !deploymentIDRegex.MatchString(deploymentID) {
						continue
					}
				} else {
					// Even for modern, ensure deployment tag matches strict format
					if !deploymentIDRegex.MatchString(deploymentID) {
						continue
					}
				}

				ownerTag := tagValue(mapping.Tags, "Owner")
				if !ownerMatchesFilter(ownerTag, ownerFilter) {
					continue
				}
				arn := awsString(mapping.ResourceARN)
				if _, seen := processed[arn]; seen {
					continue
				}
				processed[arn] = struct{}{}
				reg := parseRegionFromARN(arn)
				if reg == "" {
					reg = region
				}
				sum := summaries[deploymentID]
				if sum == nil {
					sum = &DeploymentSummary{
						ID:        deploymentID,
						Provider:  "aws",
						Region:    reg,
						Owner:     ownerTag,
						CreatedAt: time.Time{},
						State:     "unknown",
					}
					summaries[deploymentID] = sum
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
						instanceIDs[deploymentID] = append(instanceIDs[deploymentID], rID)
					}
				case ResourceEBSVolume:
					if rID != "" {
						volumeIDs[deploymentID] = append(volumeIDs[deploymentID], rID)
					}
				case ResourceSSMParam:
					if rID != "" {
						ssmNames[deploymentID] = append(ssmNames[deploymentID], rID)
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
			if out.PaginationToken == nil || *out.PaginationToken == "" {
				break
			}
			paginationToken = *out.PaginationToken
		}
		return nil
	}

	if legacy {
		// Legacy mode: Only discover via Deployment tag presence (ignore Project tag)
		if err := processWithFilters([]rgttypes.TagFilter{{Key: aws.String("Deployment")}}); err != nil {
			return nil, err
		}
	} else {
		// Default: Require Project tag to be exasol-personal
		if err := processWithFilters([]rgttypes.TagFilter{{Key: aws.String("Project"), Values: []string{"exasol-personal"}}}); err != nil {
			return nil, err
		}
	}

	// EC2 enrichment for state + earliest
	for depID, ids := range instanceIDs {
		out, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
		if err != nil {
			slog.Debug("instance describe failed", "deployment", depID, "error", err)
			continue
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
			s := summaries[depID]
			s.CreatedAt = PreferEarlier(s.CreatedAt, earliest)
		}
		switch {
		case hasActive:
			summaries[depID].State = StateActive
		case hasStopped:
			summaries[depID].State = StateStopped
		case !foundAny:
			summaries[depID].State = StateTerminated
		default:
			// no update
		}
	}

	// EBS fallback for CreatedAt
	for depID, ids := range volumeIDs {
		sum := summaries[depID]
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

	// SSM fallback for CreatedAt
	if len(ssmNames) > 0 {
		ssmClient := ssm.NewFromConfig(cfg)
		for depID, names := range ssmNames {
			sum := summaries[depID]
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

	// Mark orphaned
	for depID, sum := range summaries {
		if sum.Resources > 0 {
			if _, ok := instanceIDs[depID]; !ok {
				if sum.State == StateUnknown {
					sum.State = "orphaned"
				}
			}
		}
	}

	result := make([]DeploymentSummary, 0, len(summaries))
	for _, s := range summaries {
		result = append(result, *s)
	}

	return result, nil
}

// preferPtrEarlier returns the earlier non-nil pointer, else existing.
func preferPtrEarlier(existing *time.Time, candidate *time.Time) *time.Time {
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
