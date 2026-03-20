// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package aws

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ec2svc "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	iamsvc "github.com/aws/aws-sdk-go-v2/service/iam"
	s3svc "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	ssmsvc "github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Handler defines deletion behavior for a resource type.
type Handler interface {
	Delete(ctx context.Context, ref ResourceRef) error
}

var ErrUnsupportedResource = errors.New("unsupported resource type for deletion")

// registry maps resource types to concrete handlers.
var registry = map[ResourceType]Handler{}

// initHandlers initializes handler singletons lazily.
func initHandlers(ctx context.Context, region string) error {
	if len(registry) > 0 {
		return nil
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return err
	}
	ec2Client := ec2svc.NewFromConfig(cfg)
	ssmClient := ssmsvc.NewFromConfig(cfg)
	s3Client := s3svc.NewFromConfig(cfg)
	iamClient := iamsvc.NewFromConfig(cfg)
	registry[ResourceEC2Instance] = &ec2InstanceHandler{client: ec2Client}
	registry[ResourceEBSVolume] = &ec2VolumeHandler{client: ec2Client}
	registry[ResourceEC2KeyPair] = &ec2KeyPairHandler{client: ec2Client}
	registry[ResourceVPCEndpoint] = &ec2VpcEndpointHandler{client: ec2Client}
	registry[ResourceInternetGW] = &ec2InternetGatewayHandler{client: ec2Client}
	registry[ResourceRouteTable] = &ec2RouteTableHandler{client: ec2Client}
	registry[ResourceSecurityGrp] = &ec2SecurityGroupHandler{client: ec2Client}
	registry[ResourceSubnet] = &ec2SubnetHandler{client: ec2Client}
	registry[ResourceVPC] = &ec2VPCHandler{client: ec2Client}
	registry[ResourceSSMParam] = &ssmParamHandler{client: ssmClient}
	registry[ResourceS3Bucket] = &s3BucketHandler{client: s3Client}
	registry[ResourceIAMRole] = &iamRoleHandler{client: iamClient}
	registry[ResourceIAMInstProf] = &iamInstanceProfileHandler{client: iamClient}

	return nil
}

// Generic EC2 handlers (skeleton implementations).
type ec2InstanceHandler struct{ client *ec2svc.Client }

func (h *ec2InstanceHandler) Delete(ctx context.Context, ref ResourceRef) error {
	_, err := h.client.TerminateInstances(
		ctx,
		&ec2svc.TerminateInstancesInput{InstanceIds: []string{ref.ID}},
	)
	if err != nil {
		// Treat NotFound as success for idempotent cleanup
		if strings.Contains(err.Error(), "InvalidInstanceID.NotFound") {
			return nil
		}
	}

	return err
}

type ec2VolumeHandler struct{ client *ec2svc.Client }

func (h *ec2VolumeHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Wait for volume to be available (not attached) before deleting
	maxWaits := 30 // 30 seconds max
	for i := 0; i < maxWaits; i++ {
		descOut, err := h.client.DescribeVolumes(ctx, &ec2svc.DescribeVolumesInput{
			VolumeIds: []string{ref.ID},
		})
		if err != nil {
			if strings.Contains(err.Error(), "InvalidVolume.NotFound") {
				return nil
			}
			return err
		}
		if len(descOut.Volumes) > 0 && descOut.Volumes[0].State == ec2types.VolumeStateAvailable {
			break
		}
		time.Sleep(1 * time.Second)
	}

	_, err := h.client.DeleteVolume(ctx, &ec2svc.DeleteVolumeInput{VolumeId: aws.String(ref.ID)})
	if err != nil {
		// Treat InvalidVolume.NotFound as success for idempotency
		if strings.Contains(err.Error(), "InvalidVolume.NotFound") {
			return nil
		}
	}

	return err
}

type ec2InternetGatewayHandler struct{ client *ec2svc.Client }

func (h *ec2InternetGatewayHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Describe attachments to detach
	out, err := h.client.DescribeInternetGateways(
		ctx,
		&ec2svc.DescribeInternetGatewaysInput{InternetGatewayIds: []string{ref.ID}},
	)
	if err != nil {
		return err
	}
	// Track VPCs to clean up routes referencing the IGW
	var vpcs []string
	for _, gw := range out.InternetGateways {
		for _, att := range gw.Attachments {
			if att.VpcId != nil {
				vpcs = append(vpcs, *att.VpcId)
			}
		}
	}
	// Proactively delete routes targeting IGW before detaching,
	// as some accounts block detachment while routes exist
	for _, vpcID := range vpcs {
		rtOut, rErr := h.client.DescribeRouteTables(ctx, &ec2svc.DescribeRouteTablesInput{
			Filters: []ec2types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
		})
		if rErr != nil {
			return rErr
		}
		for _, rt := range rtOut.RouteTables {
			for _, route := range rt.Routes {
				if route.GatewayId != nil && *route.GatewayId == ref.ID && rt.RouteTableId != nil {
					input := &ec2svc.DeleteRouteInput{RouteTableId: rt.RouteTableId}
					switch {
					case route.DestinationCidrBlock != nil:
						input.DestinationCidrBlock = route.DestinationCidrBlock
					case route.DestinationIpv6CidrBlock != nil:
						input.DestinationIpv6CidrBlock = route.DestinationIpv6CidrBlock
					case route.DestinationPrefixListId != nil:
						input.DestinationPrefixListId = route.DestinationPrefixListId
					default:
						continue
					}
					if _, drErr := h.client.DeleteRoute(ctx, input); drErr != nil {
						return drErr
					}
				}
			}
		}
	}
	// Now detach IGW from each VPC
	for _, vpcID := range vpcs {
		// Retry detachment as it may fail if there are still network interfaces being released
		var detachErr error
		for i := 0; i < 10; i++ {
			_, detachErr = h.client.DetachInternetGateway(
				ctx,
				&ec2svc.DetachInternetGatewayInput{
					InternetGatewayId: aws.String(ref.ID),
					VpcId:             aws.String(vpcID),
				},
			)
			if detachErr == nil {
				break
			}
			if !strings.Contains(detachErr.Error(), "DependencyViolation") {
				return detachErr
			}
			time.Sleep(2 * time.Second)
		}
		if detachErr != nil {
			return detachErr
		}
	}
	// Wait briefly until attachments reflect detachment
	for range 5 {
		time.Sleep(1 * time.Second)
		check, cErr := h.client.DescribeInternetGateways(
			ctx,
			&ec2svc.DescribeInternetGatewaysInput{InternetGatewayIds: []string{ref.ID}},
		)
		if cErr != nil {
			break
		}
		allDetached := true
		for _, gw := range check.InternetGateways {
			for _, att := range gw.Attachments {
				if att.State == ec2types.AttachmentStatusAttached {
					allDetached = false
					break
				}
			}
		}
		if allDetached {
			break
		}
	}
	// (routes were removed before detach)
	// Retry delete IGW a few times to overcome eventual consistency
	var lastErr error
	for range igwDeleteRetries {
		_, derr := h.client.DeleteInternetGateway(
			ctx,
			&ec2svc.DeleteInternetGatewayInput{InternetGatewayId: aws.String(ref.ID)},
		)
		if derr == nil {
			return nil
		}
		lastErr = derr
		time.Sleep(igwDeleteRetryDelay)
	}

	return lastErr
}

// VPC Endpoint handler: remove route table entries referencing the endpoint, then delete the endpoint.
type ec2VpcEndpointHandler struct{ client *ec2svc.Client }

func (h *ec2VpcEndpointHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// For gateway endpoints (e.g., S3), routes reference the endpoint id in route.GatewayId.
	// Clean any routes in the VPC that target this endpoint id.
	// First, find the VPC of the endpoint to scope route tables.
	desc, err := h.client.DescribeVpcEndpoints(ctx, &ec2svc.DescribeVpcEndpointsInput{
		VpcEndpointIds: []string{ref.ID},
	})
	if err != nil {
		// If not found, treat as success
		if strings.Contains(err.Error(), "VpcEndpointIdNotFound") ||
			strings.Contains(err.Error(), "InvalidVpcEndpointId.NotFound") {
			return nil
		}
		return err
	}
	var vpcID string
	for _, ep := range desc.VpcEndpoints {
		if ep.VpcId != nil {
			vpcID = *ep.VpcId
			break
		}
	}
	// Try deleting the endpoint first. For gateway endpoints (S3) AWS manages
	// the route table entries and will remove them when the endpoint is deleted.
	// Deleting the endpoint first avoids errors like
	// "cannot remove VPC endpoint route ..." when attempting to delete routes
	// manually.
	_, delErr := h.client.DeleteVpcEndpoints(ctx, &ec2svc.DeleteVpcEndpointsInput{
		VpcEndpointIds: []string{ref.ID},
	})
	if delErr == nil {
		return nil
	}
	// Treat not found as success
	if strings.Contains(delErr.Error(), "InvalidVpcEndpointId.NotFound") ||
		strings.Contains(delErr.Error(), "VpcEndpointIdNotFound") {
		return nil
	}

	// If endpoint deletion failed for other reasons, attempt to clean up any
	// lingering route table entries referencing the endpoint, then retry
	// deletion. Some accounts may allow manual route removal as a fallback.
	if vpcID != "" {
		rtOut, rErr := h.client.DescribeRouteTables(ctx, &ec2svc.DescribeRouteTablesInput{
			Filters: []ec2types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
		})
		if rErr == nil {
			for _, rt := range rtOut.RouteTables {
				for _, route := range rt.Routes {
					if route.GatewayId != nil && *route.GatewayId == ref.ID && rt.RouteTableId != nil {
						input := &ec2svc.DeleteRouteInput{RouteTableId: rt.RouteTableId}
						switch {
						case route.DestinationCidrBlock != nil:
							input.DestinationCidrBlock = route.DestinationCidrBlock
						case route.DestinationIpv6CidrBlock != nil:
							input.DestinationIpv6CidrBlock = route.DestinationIpv6CidrBlock
						case route.DestinationPrefixListId != nil:
							input.DestinationPrefixListId = route.DestinationPrefixListId
						default:
							continue
						}
						if _, drErr := h.client.DeleteRoute(ctx, input); drErr != nil {
							// If AWS rejects manual route deletion for endpoint-managed
							// routes, ignore and continue; we'll retry endpoint delete
							// below.
							if strings.Contains(drErr.Error(), "cannot remove VPC endpoint route") {
								continue
							}
							return drErr
						}
					}
				}
			}
		}
	}

	// Retry deleting the endpoint once after attempting manual route cleanup
	_, retryErr := h.client.DeleteVpcEndpoints(ctx, &ec2svc.DeleteVpcEndpointsInput{
		VpcEndpointIds: []string{ref.ID},
	})
	if retryErr != nil {
		if strings.Contains(retryErr.Error(), "InvalidVpcEndpointId.NotFound") ||
			strings.Contains(retryErr.Error(), "VpcEndpointIdNotFound") {
			return nil
		}
	}

	// Prefer returning the original delete error if retry failed too.
	if retryErr != nil {
		return delErr
	}

	return nil
}

type ec2RouteTableHandler struct{ client *ec2svc.Client }

func (h *ec2RouteTableHandler) Delete(ctx context.Context, ref ResourceRef) error {
	out, err := h.client.DescribeRouteTables(
		ctx,
		&ec2svc.DescribeRouteTablesInput{RouteTableIds: []string{ref.ID}},
	)
	if err != nil {
		return err
	}
	for _, rt := range out.RouteTables {
		// Skip main route table (association main=true)
		for _, assoc := range rt.Associations {
			if assoc.Main != nil && *assoc.Main {
				return errors.New("cannot delete main route table: " + ref.ID)
			}
			if assoc.RouteTableAssociationId != nil {
				_, derr := h.client.DisassociateRouteTable(
					ctx,
					&ec2svc.DisassociateRouteTableInput{
						AssociationId: assoc.RouteTableAssociationId,
					},
				)
				if derr != nil {
					return derr
				}
			}
		}
	}
	_, err = h.client.DeleteRouteTable(
		ctx,
		&ec2svc.DeleteRouteTableInput{RouteTableId: aws.String(ref.ID)},
	)
	if err != nil {
		if strings.Contains(err.Error(), "InvalidRouteTableID.NotFound") {
			return nil
		}
	}

	return err
}

type ec2SecurityGroupHandler struct{ client *ec2svc.Client }

func (h *ec2SecurityGroupHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Describe to check if default
	out, err := h.client.DescribeSecurityGroups(
		ctx,
		&ec2svc.DescribeSecurityGroupsInput{GroupIds: []string{ref.ID}},
	)
	if err != nil {
		return err
	}
	for _, sg := range out.SecurityGroups {
		if sg.GroupName != nil && *sg.GroupName == "default" {
			return errors.New("skip default security group")
		}
	}

	// Retry deletion a few times to handle timing issues with ENI cleanup
	var lastErr error
	for i := 0; i < 10; i++ {
		_, err = h.client.DeleteSecurityGroup(
			ctx,
			&ec2svc.DeleteSecurityGroupInput{GroupId: aws.String(ref.ID)},
		)
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "DependencyViolation") {
			return err
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}

	return lastErr
}

type ec2SubnetHandler struct{ client *ec2svc.Client }

func (h *ec2SubnetHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Retry deletion to handle timing issues with ENI cleanup
	var lastErr error
	for i := 0; i < 10; i++ {
		_, err := h.client.DeleteSubnet(ctx, &ec2svc.DeleteSubnetInput{SubnetId: aws.String(ref.ID)})
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "DependencyViolation") {
			return err
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

type ec2VPCHandler struct{ client *ec2svc.Client }

func (h *ec2VPCHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Retry deletion to handle timing issues with resource cleanup
	var lastErr error
	for i := 0; i < 10; i++ {
		_, err := h.client.DeleteVpc(ctx, &ec2svc.DeleteVpcInput{VpcId: aws.String(ref.ID)})
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "DependencyViolation") {
			return err
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return lastErr
}

type ec2KeyPairHandler struct{ client *ec2svc.Client }

func (h *ec2KeyPairHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Support deletion by KeyPairId
	// (preferred when ref.ID looks like "key-xxxxxxxx") or by KeyName.
	input := &ec2svc.DeleteKeyPairInput{}
	if strings.HasPrefix(ref.ID, "key-") {
		input.KeyPairId = aws.String(ref.ID)
	} else {
		input.KeyName = aws.String(ref.ID)
	}
	_, err := h.client.DeleteKeyPair(ctx, input)
	if err != nil {
		// Treat NotFound as success for idempotent cleanup
		if strings.Contains(err.Error(), "InvalidKeyPair.NotFound") {
			return nil
		}
		return err
	}

	return nil
}

type ssmParamHandler struct{ client *ssmsvc.Client }

func (h *ssmParamHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Use ref.ID as-is. Our ARN classifier already returns the full parameter path
	// after the "parameter/" prefix, so additional normalization isn't necessary.
	_, err := h.client.DeleteParameter(ctx, &ssmsvc.DeleteParameterInput{Name: aws.String(ref.ID)})
	if err != nil {
		// Treat ParameterNotFound as success to make deletes idempotent
		// SDK v2 surfaces error via smithy; fall back to string contains check
		if strings.Contains(err.Error(), "ParameterNotFound") {
			return nil
		}
	}

	return err
}

// s3 bucket handler: empties objects/versions then deletes bucket; NoSuchBucket treated as success.
type s3BucketHandler struct{ client *s3svc.Client }

func (h *s3BucketHandler) Delete(ctx context.Context, ref ResourceRef) error {
	bucket := ref.ID
	// Try to delete all object versions first (handles versioned buckets)
	// List object versions
	listVers, err := h.client.ListObjectVersions(
		ctx,
		&s3svc.ListObjectVersionsInput{Bucket: aws.String(bucket)},
	)
	// nolint:nestif // nested flow is acceptable for batch deletions
	if err == nil {
		// Collect all keys + versionIds including delete markers
		toDelete := make([]s3types.ObjectIdentifier, 0)
		for _, v := range listVers.Versions {
			if v.Key != nil && v.VersionId != nil {
				toDelete = append(
					toDelete,
					s3types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId},
				)
			}
		}
		for _, d := range listVers.DeleteMarkers {
			if d.Key != nil && d.VersionId != nil {
				toDelete = append(
					toDelete,
					s3types.ObjectIdentifier{Key: d.Key, VersionId: d.VersionId},
				)
			}
		}
		if len(toDelete) > 0 {
			// batch delete in chunks of 1000 per AWS limits
			for item := 0; item < len(toDelete); item += s3BatchDeleteSize {
				end := item + s3BatchDeleteSize
				if end > len(toDelete) {
					end = len(toDelete)
				}
				_, derr := h.client.DeleteObjects(ctx, &s3svc.DeleteObjectsInput{
					Bucket: aws.String(bucket),
					Delete: &s3types.Delete{Objects: toDelete[item:end], Quiet: aws.Bool(true)},
				})
				if derr != nil {
					return derr
				}
			}
		}
	}
	// Non-versioned objects
	list, err := h.client.ListObjectsV2(ctx, &s3svc.ListObjectsV2Input{Bucket: aws.String(bucket)})
	// nolint:nestif // nested flow is acceptable for batch deletions
	if err == nil {
		objs := make([]s3types.ObjectIdentifier, 0)
		for _, o := range list.Contents {
			if o.Key != nil {
				objs = append(objs, s3types.ObjectIdentifier{Key: o.Key})
			}
		}
		if len(objs) > 0 {
			for item := 0; item < len(objs); item += s3BatchDeleteSize {
				end := item + s3BatchDeleteSize
				if end > len(objs) {
					end = len(objs)
				}
				_, derr := h.client.DeleteObjects(ctx, &s3svc.DeleteObjectsInput{
					Bucket: aws.String(bucket),
					Delete: &s3types.Delete{Objects: objs[item:end], Quiet: aws.Bool(true)},
				})
				if derr != nil {
					return derr
				}
			}
		}
	}
	// Attempt bucket delete
	_, err = h.client.DeleteBucket(ctx, &s3svc.DeleteBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucket") {
			return nil
		}
	}

	return err
}

// deleteResource dispatches deletion to the appropriate handler.
func deleteResource(ctx context.Context, region string, ref ResourceRef) error {
	if err := initHandlers(ctx, region); err != nil {
		return err
	}
	h, ok := registry[ref.Type]
	if !ok {
		return ErrUnsupportedResource
	}

	return h.Delete(ctx, ref)
}

// IAM handlers
type iamRoleHandler struct{ client *iamsvc.Client }

func (h *iamRoleHandler) Delete(ctx context.Context, ref ResourceRef) error {
	roleName := ref.ID
	// Detach attached policies
	attached, err := h.client.ListAttachedRolePolicies(ctx, &iamsvc.ListAttachedRolePoliciesInput{RoleName: aws.String(roleName)})
	if err == nil {
		for _, ap := range attached.AttachedPolicies {
			if ap.PolicyArn != nil {
				_, _ = h.client.DetachRolePolicy(ctx, &iamsvc.DetachRolePolicyInput{
					RoleName:  aws.String(roleName),
					PolicyArn: ap.PolicyArn,
				})
			}
		}
	}
	// Delete inline policies
	inlines, err := h.client.ListRolePolicies(ctx, &iamsvc.ListRolePoliciesInput{RoleName: aws.String(roleName)})
	if err == nil {
		for _, pn := range inlines.PolicyNames {
			_, _ = h.client.DeleteRolePolicy(ctx, &iamsvc.DeleteRolePolicyInput{
				RoleName:   aws.String(roleName),
				PolicyName: aws.String(pn),
			})
		}
	}
	// Finally delete role
	_, derr := h.client.DeleteRole(ctx, &iamsvc.DeleteRoleInput{RoleName: aws.String(roleName)})
	if derr != nil {
		if strings.Contains(derr.Error(), "NoSuchEntity") {
			return nil
		}
	}

	return derr
}

type iamInstanceProfileHandler struct{ client *iamsvc.Client }

func (h *iamInstanceProfileHandler) Delete(ctx context.Context, ref ResourceRef) error {
	name := ref.ID
	// Remove roles from instance profile
	prof, err := h.client.GetInstanceProfile(ctx, &iamsvc.GetInstanceProfileInput{InstanceProfileName: aws.String(name)})
	if err == nil && prof.InstanceProfile != nil {
		for _, role := range prof.InstanceProfile.Roles {
			if role.RoleName != nil {
				_, _ = h.client.RemoveRoleFromInstanceProfile(ctx, &iamsvc.RemoveRoleFromInstanceProfileInput{
					InstanceProfileName: aws.String(name),
					RoleName:            role.RoleName,
				})
			}
		}
	}
	// Delete the instance profile itself
	_, derr := h.client.DeleteInstanceProfile(ctx, &iamsvc.DeleteInstanceProfileInput{InstanceProfileName: aws.String(name)})
	if derr != nil {
		if strings.Contains(derr.Error(), "NoSuchEntity") {
			return nil
		}
	}

	return derr
}
