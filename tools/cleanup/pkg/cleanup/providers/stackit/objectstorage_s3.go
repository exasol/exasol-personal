// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	objectstorage "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api"
)

const (
	s3BatchDeleteSize                   = 1000
	stackitTempBucketCredsMaxNameLength = 32
	stackitTempBucketCredsPrefix        = "exa-cln-"
)

func (h *handler) newTemporaryObjectStorageS3Client(ctx context.Context) (*s3.Client, func(), error) {
	temporaryGroupName := temporaryObjectStorageCredentialsGroupName(time.Now())
	groupPayload := objectstorage.NewCreateCredentialsGroupPayload(temporaryGroupName)
	groupResp, err := h.objectStorageClient.DefaultAPI.CreateCredentialsGroup(ctx, h.projectId, h.region).
		CreateCredentialsGroupPayload(*groupPayload).
		Execute()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temporary credentials group: %w", err)
	}

	credentialsGroup := groupResp.GetCredentialsGroup()
	groupID := credentialsGroup.GetCredentialsGroupId()
	accessKeyPayload := objectstorage.NewCreateAccessKeyPayload()
	accessKeyResp, err := h.objectStorageClient.DefaultAPI.CreateAccessKey(ctx, h.projectId, h.region).
		CreateAccessKeyPayload(*accessKeyPayload).
		CredentialsGroup(groupID).
		Execute()
	if err != nil {
		_, _ = h.objectStorageClient.DefaultAPI.DeleteCredentialsGroup(context.Background(), h.projectId, h.region, groupID).Execute()
		return nil, nil, fmt.Errorf("failed to create temporary access key: %w", err)
	}

	endpoint := fmt.Sprintf("https://object.storage.%s.onstackit.cloud", h.region)
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(h.region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyResp.GetAccessKey(),
			accessKeyResp.GetSecretAccessKey(),
			"",
		)),
		awsconfig.WithEndpointResolverWithOptions(awssdk.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (awssdk.Endpoint, error) {
				return awssdk.Endpoint{
					URL:               endpoint,
					HostnameImmutable: true,
					SigningRegion:     h.region,
				}, nil
			},
		)),
	)
	if err != nil {
		_ = h.deleteObjectStorageAccessKey(context.Background(), accessKeyResp.GetKeyId(), groupID)
		_, _ = h.objectStorageClient.DefaultAPI.DeleteCredentialsGroup(context.Background(), h.projectId, h.region, groupID).Execute()
		return nil, nil, fmt.Errorf("failed to configure temporary S3 client: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.UsePathStyle = true
	})

	cleanup := func() {
		cleanupCtx := context.Background()
		_ = h.deleteObjectStorageAccessKey(cleanupCtx, accessKeyResp.GetKeyId(), groupID)
		_, _ = h.objectStorageClient.DefaultAPI.DeleteCredentialsGroup(cleanupCtx, h.projectId, h.region, groupID).Execute()
	}

	return client, cleanup, nil
}

func temporaryObjectStorageCredentialsGroupName(now time.Time) string {
	name := fmt.Sprintf("%s%x", stackitTempBucketCredsPrefix, now.UTC().UnixNano())
	if len(name) <= stackitTempBucketCredsMaxNameLength {
		return name
	}

	return name[:stackitTempBucketCredsMaxNameLength]
}

func emptyObjectStorageBucket(ctx context.Context, client *s3.Client, bucket string) error {
	continuationToken := (*string)(nil)
	for {
		listInput := &s3.ListObjectsV2Input{Bucket: awssdk.String(bucket)}
		if continuationToken != nil {
			listInput.ContinuationToken = continuationToken
		}

		listResp, err := client.ListObjectsV2(ctx, listInput)
		if err != nil {
			if isS3NotFoundError(err) {
				return nil
			}
			return fmt.Errorf("failed to list bucket objects: %w", err)
		}

		objects := make([]s3types.ObjectIdentifier, 0, len(listResp.Contents))
		for _, object := range listResp.Contents {
			if object.Key != nil {
				objects = append(objects, s3types.ObjectIdentifier{Key: object.Key})
			}
		}

		for start := 0; start < len(objects); start += s3BatchDeleteSize {
			end := start + s3BatchDeleteSize
			if end > len(objects) {
				end = len(objects)
			}

			_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: awssdk.String(bucket),
				Delete: &s3types.Delete{Objects: objects[start:end], Quiet: awssdk.Bool(true)},
			})
			if err != nil {
				return fmt.Errorf("failed to delete bucket objects: %w", err)
			}
		}

		if !awssdk.ToBool(listResp.IsTruncated) || listResp.NextContinuationToken == nil {
			return nil
		}

		continuationToken = listResp.NextContinuationToken
	}
}

func isS3NotFoundError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "nosuchbucket") || strings.Contains(message, "not found")
}
