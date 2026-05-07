// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	iaas "github.com/stackitcloud/stackit-sdk-go/services/iaas/v2api"
	iaasWait "github.com/stackitcloud/stackit-sdk-go/services/iaas/v2api/wait"
	objectstorage "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api"
	objectstorageWait "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api/wait"
)

var ErrUnsupportedResource = errors.New("unsupported resource type for deletion")

var iaasClient *iaas.APIClient
var objectStorageClient *objectstorage.APIClient

func deleteResource(ctx context.Context, projectId, region string, ref ResourceRef) error {
	if iaasClient == nil {
		iaasC, osC, _, err := createStackitClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create STACKIT client: %w", err)
		}

		iaasClient = iaasC
		objectStorageClient = osC
	}

	h := handler{iaasClient: iaasClient, objectStorageClient: objectStorageClient, projectId: projectId, region: region}

	deleters := map[ResourceType]func(context.Context, ResourceRef) error{
		ResourceServer:                        h.DeleteServer,
		ResourceVolume:                        h.DeleteVolume,
		ResourceNetwork:                       h.DeleteNetwork,
		ResourceSecurityGroup:                 h.DeleteSecurityGroup,
		ResourceObjectStorageBucket:           h.DeleteObjectStorageBucket,
		ResourceObjectStorageCredential:       h.DeleteObjectStorageCredential,
		ResourceObjectStorageCredentialsGroup: h.DeleteObjectStorageCredentialsGroup,
	}

	deleter, ok := deleters[ref.Type]
	if ok {
		return deleter(ctx, ref)
	}

	return ErrUnsupportedResource
}

type handler struct {
	iaasClient          *iaas.APIClient
	objectStorageClient *objectstorage.APIClient
	projectId           string
	region              string
}

func (h *handler) DeleteServer(ctx context.Context, ref ResourceRef) error {
	err := h.iaasClient.DefaultAPI.DeleteServer(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete server: %w", err)
	}

	_, err = iaasWait.DeleteServerWaitHandler(context.Background(), h.iaasClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(context.Background())
	if err != nil {
		return fmt.Errorf("failed to wait for server deletion: %w", err)
	}

	return nil
}

func (h *handler) DeleteVolume(ctx context.Context, ref ResourceRef) error {
	err := h.iaasClient.DefaultAPI.DeleteVolume(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	_, err = iaasWait.DeleteVolumeWaitHandler(context.Background(), h.iaasClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(context.Background())
	if err != nil {
		return fmt.Errorf("failed to wait for volume deletion: %w", err)
	}

	return nil
}

func (h *handler) DeleteSecurityGroup(ctx context.Context, ref ResourceRef) error {
	err := h.iaasClient.DefaultAPI.DeleteSecurityGroup(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete security group: %w", err)
	}

	return nil
}

func (h *handler) DeleteNetwork(ctx context.Context, ref ResourceRef) error {
	err := h.iaasClient.DefaultAPI.DeleteNetwork(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	_, err = iaasWait.DeleteNetworkWaitHandler(context.Background(), h.iaasClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(context.Background())
	if err != nil {
		return fmt.Errorf("failed to wait for network deletion: %w", err)
	}

	return nil
}

func (h *handler) DeleteObjectStorageBucket(ctx context.Context, ref ResourceRef) error {
	_, err := h.objectStorageClient.DefaultAPI.DeleteBucket(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	_, err = objectstorageWait.DeleteBucketWaitHandler(context.Background(), h.objectStorageClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(context.Background())
	if err != nil {
		return fmt.Errorf("failed to wait for bucket deletion: %w", err)
	}
	return nil
}

func (h *handler) DeleteObjectStorageCredential(ctx context.Context, ref ResourceRef) error {
	_, err := h.objectStorageClient.DefaultAPI.DeleteAccessKey(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	return nil
}

func (h *handler) DeleteObjectStorageCredentialsGroup(ctx context.Context, ref ResourceRef) error {
	_, err := h.objectStorageClient.DefaultAPI.DeleteCredentialsGroup(context.Background(), h.projectId, h.region, ref.ID).Execute()
	if err != nil {
		// Treat not found as success for idempotent cleanup
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	return nil
}
