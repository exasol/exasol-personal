// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	v3 "github.com/exoscale/egoscale/v3"
)

// Handler defines deletion behavior for a resource type.
type Handler interface {
	Delete(ctx context.Context, ref ResourceRef) error
}

var ErrUnsupportedResource = errors.New("unsupported resource type for deletion")

// registry maps resource types to concrete handlers.
var registry = map[ResourceType]Handler{}

// initHandlers initializes handler singletons lazily.
func initHandlers(ctx context.Context, zone string) error {
	if len(registry) > 0 {
		return nil
	}
	
	client, err := createExoscaleClient(ctx, zone)
	if err != nil {
		return fmt.Errorf("failed to create Exoscale client: %w", err)
	}
	
	// Initialize handlers
	registry[ResourceComputeInstance] = &computeInstanceHandler{client: client, zone: zone}
	registry[ResourceBlockVolume] = &blockStorageVolumeHandler{client: client, zone: zone}
	registry[ResourcePrivateNetwork] = &privateNetworkHandler{client: client, zone: zone}
	registry[ResourceSecurityGroup] = &securityGroupHandler{client: client, zone: zone}
	registry[ResourceSSHKey] = &sshKeyHandler{client: client, zone: zone}
	registry[ResourceIAMRole] = &iamRoleHandler{client: client}
	registry[ResourceIAMAPIKey] = &iamAPIKeyHandler{client: client}
	registry[ResourceSOSBucket] = &sosBucketHandler{zone: zone}

	return nil
}

func deleteResource(ctx context.Context, zone string, ref ResourceRef) error {
	if err := initHandlers(ctx, zone); err != nil {
		return err
	}
	h, ok := registry[ref.Type]
	if !ok {
		return ErrUnsupportedResource
	}

	return h.Delete(ctx, ref)
}

// Compute Instance Handler
type computeInstanceHandler struct {
	client *v3.Client
	zone   string
}

func (h *computeInstanceHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Parse UUID from ID string
	uuid, err := v3.ParseUUID(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %w", err)
	}
	
	op, err := h.client.DeleteInstance(ctx, uuid)
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	
	// Wait for operation to complete
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for instance deletion: %w", err)
	}
	
	return nil
}

// Block Storage Volume Handler
type blockStorageVolumeHandler struct {
	client *v3.Client
	zone   string
}

func (h *blockStorageVolumeHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Parse UUID from ID string
	uuid, err := v3.ParseUUID(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid volume ID: %w", err)
	}
	
	// Wait for volume to be detached before deleting
	maxWaits := 60 // 60 seconds max
	for i := 0; i < maxWaits; i++ {
		vol, err := h.client.GetBlockStorageVolume(ctx, uuid)
		if err != nil {
			// Volume not found is OK
			if strings.Contains(err.Error(), "404") {
				return nil
			}
			return fmt.Errorf("failed to check volume status: %w", err)
		}
		
		if vol.State == v3.BlockStorageVolumeStateDetached || 
			vol.State == "available" {
			goto readyToDelete
		}
		
		time.Sleep(1 * time.Second)
	}
	
readyToDelete:
	op, err := h.client.DeleteBlockStorageVolume(ctx, uuid)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("failed to delete volume: %w", err)
	}
	
	// Wait for operation to complete
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for volume deletion: %w", err)
	}
	
	return nil
}

// Private Network Handler
type privateNetworkHandler struct {
	client *v3.Client
	zone   string
}

func (h *privateNetworkHandler) Delete(ctx context.Context, ref ResourceRef) error {
	uuid, err := v3.ParseUUID(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid network ID: %w", err)
	}
	
	op, err := h.client.DeletePrivateNetwork(ctx, uuid)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete private network: %w", err)
	}
	
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for network deletion: %w", err)
	}
	
	return nil
}

// Security Group Handler
type securityGroupHandler struct {
	client *v3.Client
	zone   string
}

func (h *securityGroupHandler) Delete(ctx context.Context, ref ResourceRef) error {
	uuid, err := v3.ParseUUID(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid security group ID: %w", err)
	}
	
	op, err := h.client.DeleteSecurityGroup(ctx, uuid)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete security group: %w", err)
	}
	
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for security group deletion: %w", err)
	}
	
	return nil
}

// SSH Key Handler
type sshKeyHandler struct {
	client *v3.Client
	zone   string
}

func (h *sshKeyHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// SSH keys use name as ID in v3
	op, err := h.client.DeleteSSHKey(ctx, ref.ID)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete SSH key: %w", err)
	}
	
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for SSH key deletion: %w", err)
	}
	
	return nil
}

// IAM Role Handler
type iamRoleHandler struct {
	client *v3.Client
}

func (h *iamRoleHandler) Delete(ctx context.Context, ref ResourceRef) error {
	uuid, err := v3.ParseUUID(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid IAM role ID: %w", err)
	}
	
	op, err := h.client.DeleteIAMRole(ctx, uuid)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete IAM role: %w", err)
	}
	
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for IAM role deletion: %w", err)
	}
	
	return nil
}

// IAM API Key Handler
type iamAPIKeyHandler struct {
	client *v3.Client
}

func (h *iamAPIKeyHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// API keys use the key string as ID
	op, err := h.client.DeleteAPIKey(ctx, ref.ID)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	
	_, err = h.client.Wait(ctx, op, v3.OperationStateSuccess)
	if err != nil {
		return fmt.Errorf("failed to wait for API key deletion: %w", err)
	}
	
	return nil
}

// SOS Bucket Handler (uses S3 API)
type sosBucketHandler struct {
	zone string
}

func (h *sosBucketHandler) Delete(ctx context.Context, ref ResourceRef) error {
	sosEndpoint := fmt.Sprintf("https://sos-%s.exo.io", h.zone)
	
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(h.zone),
		awsconfig.WithEndpointResolverWithOptions(awssdk.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (awssdk.Endpoint, error) {
				return awssdk.Endpoint{
					URL:               sosEndpoint,
					HostnameImmutable: true,
					SigningRegion:     h.zone,
				}, nil
			},
		)),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config for SOS: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	
	// List and delete all objects in the bucket first
	listResp, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: awssdk.String(ref.ID),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucket") {
			return nil
		}
		return fmt.Errorf("failed to list bucket objects: %w", err)
	}
	
	// Delete all objects
	for _, obj := range listResp.Contents {
		_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: awssdk.String(ref.ID),
			Key:    obj.Key,
		})
		if err != nil {
			return fmt.Errorf("failed to delete object %s: %w", *obj.Key, err)
		}
	}
	
	// Delete the bucket
	_, err = s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: awssdk.String(ref.ID),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucket") {
			return nil
		}
		return fmt.Errorf("failed to delete bucket: %w", err)
	}
	
	return nil
}
