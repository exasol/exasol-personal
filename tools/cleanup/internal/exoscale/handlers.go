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
	egoscale "github.com/exoscale/egoscale/v2"
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
	
	apiCli, err := newAPIClient(zone)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}
	
	// Initialize handlers
	registry[ResourceComputeInstance] = &computeInstanceHandler{client: client, zone: zone}
	registry[ResourceBlockVolume] = &blockStorageVolumeHandler{apiClient: apiCli, zone: zone}
	registry[ResourcePrivateNetwork] = &privateNetworkHandler{client: client, zone: zone}
	registry[ResourceSecurityGroup] = &securityGroupHandler{client: client, zone: zone}
	registry[ResourceSSHKey] = &sshKeyHandler{client: client, zone: zone}
	registry[ResourceIAMRole] = &iamRoleHandler{client: client, zone: zone}
	registry[ResourceIAMAPIKey] = &iamAPIKeyHandler{apiClient: apiCli}
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
	client *egoscale.Client
	zone   string
}

func (h *computeInstanceHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Create instance struct with just the ID
	instance := &egoscale.Instance{
		ID: &ref.ID,
	}
	
	err := h.client.DeleteInstance(ctx, h.zone, instance)
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	
	return nil
}

// Block Storage Volume Handler (uses direct API)
type blockStorageVolumeHandler struct {
	apiClient *apiClient
	zone      string
}

func (h *blockStorageVolumeHandler) Delete(ctx context.Context, ref ResourceRef) error {
	// Wait for volume to be detached before deleting
	maxWaits := 60 // 60 seconds max
	for i := 0; i < maxWaits; i++ {
		volumes, err := h.apiClient.listBlockStorageVolumes(ctx)
		if err != nil {
			return fmt.Errorf("failed to check volume status: %w", err)
		}
		
		found := false
		for _, vol := range volumes {
			if vol.ID == ref.ID {
				found = true
				if vol.State == "detached" || vol.State == "available" {
					goto readyToDelete
				}
				break
			}
		}
		
		if !found {
			// Volume already deleted
			return nil
		}
		
		time.Sleep(1 * time.Second)
	}
	
readyToDelete:
	respBody, err := h.apiClient.doRequest(ctx, "DELETE", "/block-storage/"+ref.ID, nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("failed to delete volume: %w", err)
	}
	
	// Response is an operation object, we don't wait for it to complete
	_ = respBody
	
	return nil
}

// Private Network Handler
type privateNetworkHandler struct {
	client *egoscale.Client
	zone   string
}

func (h *privateNetworkHandler) Delete(ctx context.Context, ref ResourceRef) error {
	network := &egoscale.PrivateNetwork{
		ID: &ref.ID,
	}
	
	err := h.client.DeletePrivateNetwork(ctx, h.zone, network)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete private network: %w", err)
	}
	
	return nil
}

// Security Group Handler
type securityGroupHandler struct {
	client *egoscale.Client
	zone   string
}

func (h *securityGroupHandler) Delete(ctx context.Context, ref ResourceRef) error {
	securityGroup := &egoscale.SecurityGroup{
		ID: &ref.ID,
	}
	
	err := h.client.DeleteSecurityGroup(ctx, h.zone, securityGroup)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete security group: %w", err)
	}
	
	return nil
}

// SSH Key Handler
type sshKeyHandler struct {
	client *egoscale.Client
	zone   string
}

func (h *sshKeyHandler) Delete(ctx context.Context, ref ResourceRef) error {
	sshKey := &egoscale.SSHKey{
		Name: &ref.ID, // SSH keys use name as ID
	}
	
	err := h.client.DeleteSSHKey(ctx, h.zone, sshKey)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete SSH key: %w", err)
	}
	
	return nil
}

// IAM Role Handler
type iamRoleHandler struct {
	client *egoscale.Client
	zone   string
}

func (h *iamRoleHandler) Delete(ctx context.Context, ref ResourceRef) error {
	iamRole := &egoscale.IAMRole{
		ID: &ref.ID,
	}
	
	err := h.client.DeleteIAMRole(ctx, h.zone, iamRole)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete IAM role: %w", err)
	}
	
	return nil
}

// IAM API Key Handler (uses direct API)
type iamAPIKeyHandler struct {
	apiClient *apiClient
}

func (h *iamAPIKeyHandler) Delete(ctx context.Context, ref ResourceRef) error {
	respBody, err := h.apiClient.doRequest(ctx, "DELETE", "/api-key/"+ref.ID, nil, nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	
	// Response is an operation object, we don't wait for it to complete
	_ = respBody
	
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
