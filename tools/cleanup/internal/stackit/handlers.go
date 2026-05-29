// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	iaas "github.com/stackitcloud/stackit-sdk-go/services/iaas/v2api"
	iaasWait "github.com/stackitcloud/stackit-sdk-go/services/iaas/v2api/wait"
	objectstorage "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api"
	objectstorageWait "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api/wait"
)

var ErrUnsupportedResource = errors.New("unsupported resource type for deletion")

const (
	stackitDeleteRetryCount = 15
	stackitDeleteRetryDelay = 2 * time.Second
	stackitBucketRetryCount = 3
	stackitBucketRetryDelay = 2 * time.Second
)

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
		ResourcePublicIP:                      h.DeletePublicIP,
		ResourceNetworkInterface:              h.DeleteNetworkInterface,
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
	deleted, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		return h.iaasClient.DefaultAPI.DeleteServer(ctx, h.projectId, h.region, ref.ID).Execute()
	})
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}
	if !deleted {
		return nil
	}

	_, err = iaasWait.DeleteServerWaitHandler(ctx, h.iaasClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to wait for server deletion: %w", err)
	}

	return nil
}

func (h *handler) DeleteVolume(ctx context.Context, ref ResourceRef) error {
	deleted, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		return h.iaasClient.DefaultAPI.DeleteVolume(ctx, h.projectId, h.region, ref.ID).Execute()
	})
	if err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}
	if !deleted {
		return nil
	}

	_, err = iaasWait.DeleteVolumeWaitHandler(ctx, h.iaasClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to wait for volume deletion: %w", err)
	}

	return nil
}

func (h *handler) DeletePublicIP(ctx context.Context, ref ResourceRef) error {
	_, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		return h.iaasClient.DefaultAPI.DeletePublicIP(ctx, h.projectId, h.region, ref.ID).Execute()
	})
	if err != nil {
		return fmt.Errorf("failed to delete public IP: %w", err)
	}

	return nil
}

func (h *handler) DeleteNetworkInterface(ctx context.Context, ref ResourceRef) error {
	if ref.ParentID == "" {
		return errors.New("failed to delete network interface: missing parent network id")
	}

	_, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		return h.iaasClient.DefaultAPI.DeleteNic(ctx, h.projectId, h.region, ref.ParentID, ref.ID).Execute()
	})
	if err != nil {
		return fmt.Errorf("failed to delete network interface: %w", err)
	}

	return nil
}

func (h *handler) DeleteSecurityGroup(ctx context.Context, ref ResourceRef) error {
	_, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		return h.iaasClient.DefaultAPI.DeleteSecurityGroup(ctx, h.projectId, h.region, ref.ID).Execute()
	})
	if err != nil {
		return fmt.Errorf("failed to delete security group: %w", err)
	}

	return nil
}

func (h *handler) DeleteNetwork(ctx context.Context, ref ResourceRef) error {
	deleted, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		return h.iaasClient.DefaultAPI.DeleteNetwork(ctx, h.projectId, h.region, ref.ID).Execute()
	})
	if err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}
	if !deleted {
		return nil
	}

	_, err = iaasWait.DeleteNetworkWaitHandler(ctx, h.iaasClient.DefaultAPI, h.projectId, h.region, ref.ID).WaitWithContext(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to wait for network deletion: %w", err)
	}

	return nil
}

func (h *handler) DeleteObjectStorageBucket(ctx context.Context, ref ResourceRef) error {
	err := h.deleteObjectStorageBucketOnce(ctx, ref.ID)
	if err == nil {
		return nil
	}
	if !isBucketNotEmptyError(err) {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	s3Client, cleanup, err := h.newTemporaryObjectStorageS3Client(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare temporary object storage credentials: %w", err)
	}
	defer cleanup()

	for attempt := 0; attempt < stackitBucketRetryCount; attempt++ {
		if err := emptyObjectStorageBucket(ctx, s3Client, ref.ID); err != nil {
			return fmt.Errorf("failed to empty bucket: %w", err)
		}

		deleteErr := h.deleteObjectStorageBucketOnce(ctx, ref.ID)
		if deleteErr == nil {
			return nil
		}

		if !isBucketNotEmptyError(deleteErr) || attempt == stackitBucketRetryCount-1 {
			return fmt.Errorf("failed to delete bucket: %w", deleteErr)
		}

		if err := sleepWithContext(ctx, stackitBucketRetryDelay); err != nil {
			return err
		}
	}

	return nil
}

func (h *handler) deleteObjectStorageBucketOnce(ctx context.Context, bucketName string) error {
	deleted, err := deleteWithRetry(ctx, stackitDeleteRetryCount, stackitDeleteRetryDelay, func() error {
		_, err := h.objectStorageClient.DefaultAPI.DeleteBucket(ctx, h.projectId, h.region, bucketName).Execute()
		return err
	})
	if err != nil {
		return err
	}
	if !deleted {
		return nil
	}

	_, err = objectstorageWait.DeleteBucketWaitHandler(ctx, h.objectStorageClient.DefaultAPI, h.projectId, h.region, bucketName).WaitWithContext(ctx)
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to wait for bucket deletion: %w", err)
	}

	return nil
}

func (h *handler) DeleteObjectStorageCredential(ctx context.Context, ref ResourceRef) error {
	err := h.deleteObjectStorageAccessKey(ctx, ref.ID, ref.ParentID)
	if err != nil {
		return fmt.Errorf("failed to delete object storage credential: %w", err)
	}

	return nil
}

func (h *handler) DeleteObjectStorageCredentialsGroup(ctx context.Context, ref ResourceRef) error {
	_, err := h.objectStorageClient.DefaultAPI.DeleteCredentialsGroup(ctx, h.projectId, h.region, ref.ID).Execute()
	if err == nil || isNotFoundError(err) {
		return nil
	}

	if !isActiveAccessKeysError(err) {
		return fmt.Errorf("failed to delete object storage credential group: %w", err)
	}

	deletedCount, cleanupErr := h.deleteMatchingCredentialsGroupAccessKeys(ctx, ref.ID)
	if cleanupErr != nil {
		return fmt.Errorf("failed to delete object storage credential group: %w", errors.Join(err, cleanupErr))
	}
	if deletedCount == 0 {
		return fmt.Errorf("failed to delete object storage credential group: %w", err)
	}

	_, err = h.objectStorageClient.DefaultAPI.DeleteCredentialsGroup(ctx, h.projectId, h.region, ref.ID).Execute()
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete object storage credential group: %w", err)
	}

	return nil
}

func (h *handler) deleteMatchingCredentialsGroupAccessKeys(ctx context.Context, groupID string) (int, error) {
	accessKeysResp, err := h.objectStorageClient.DefaultAPI.ListAccessKeys(ctx, h.projectId, h.region).
		CredentialsGroup(groupID).
		Execute()
	if err != nil {
		if isNotFoundError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list object storage credentials: %w", err)
	}

	deletedCount := 0
	for _, accessKey := range accessKeysResp.GetAccessKeys() {
		err := h.deleteObjectStorageAccessKey(ctx, accessKey.GetKeyId(), groupID)
		if err != nil {
			return deletedCount, fmt.Errorf("failed to delete object storage credential %s: %w", accessKey.GetKeyId(), err)
		}

		deletedCount++
	}

	return deletedCount, nil
}

func (h *handler) deleteObjectStorageAccessKey(ctx context.Context, keyID, groupID string) error {
	request := h.objectStorageClient.DefaultAPI.DeleteAccessKey(ctx, h.projectId, h.region, keyID)
	if groupID != "" {
		request = request.CredentialsGroup(groupID)
	}

	_, err := request.Execute()
	if err != nil && !isNotFoundError(err) {
		return err
	}

	return nil
}

func deleteWithRetry(
	ctx context.Context,
	attempts int,
	delay time.Duration,
	deleter func() error,
) (bool, error) {
	for attempt := 0; attempt < attempts; attempt++ {
		err := deleter()
		if err == nil {
			return true, nil
		}
		if isNotFoundError(err) {
			return false, nil
		}
		if !isConflictError(err) || attempt == attempts-1 {
			return false, err
		}

		if err := sleepWithContext(ctx, delay); err != nil {
			return false, err
		}
	}

	return false, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found")
}

func isConflictError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "409") || strings.Contains(message, "conflict") || strings.Contains(message, "still in use") || strings.Contains(message, " in use")
}

func isBucketNotEmptyError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bucket.not_empty") || strings.Contains(message, "bucket is not empty")
}

func isActiveAccessKeysError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "active_access_keys") || strings.Contains(message, "actively used access keys")
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
