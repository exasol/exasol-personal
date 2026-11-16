// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestNewSOSS3ClientIgnoresAmbientAWSCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ambient-aws-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-aws-secret")
	t.Setenv("AWS_SESSION_TOKEN", "ambient-aws-session-token")
	t.Setenv("EXOSCALE_API_KEY", "exoscale-key")
	t.Setenv("EXOSCALE_API_SECRET", "exoscale-secret")

	ctx := context.Background()

	client, err := newSOSS3Client(ctx, "ch-gva-2")
	if err != nil {
		t.Fatalf("newSOSS3Client returned error: %v", err)
	}

	creds, err := client.Options().Credentials.Retrieve(ctx)
	if err != nil {
		t.Fatalf("failed to resolve credentials from client: %v", err)
	}

	if creds.AccessKeyID != "exoscale-key" {
		t.Errorf("AccessKeyID = %q, want %q (ambient AWS credentials leaked into the SOS client)",
			creds.AccessKeyID, "exoscale-key")
	}
	if creds.SecretAccessKey != "exoscale-secret" {
		t.Errorf("SecretAccessKey = %q, want %q (ambient AWS credentials leaked into the SOS client)",
			creds.SecretAccessKey, "exoscale-secret")
	}
}

func TestNewSOSS3ClientRequiresExoscaleCredentials(t *testing.T) {
	t.Setenv("EXOSCALE_API_KEY", "")
	t.Setenv("EXOSCALE_API_SECRET", "")

	if _, err := newSOSS3Client(context.Background(), "ch-gva-2"); err == nil {
		t.Fatal("expected an error when Exoscale credentials are missing, got nil")
	}
}

func TestAddSOSBucketsToSummariesDiscoversBucketOnlyDeployments(t *testing.T) {
	// Given the buckets of one deployment, an unrelated bucket, and a bucket in another zone
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	buckets := []s3types.Bucket{
		{Name: awssdk.String("exasol-e59be4f0-sos-archive"), BucketRegion: awssdk.String("ch-gva-2"), CreationDate: awssdk.Time(created.Add(time.Minute))},
		{Name: awssdk.String("exasol-e59be4f0-boostrap"), BucketRegion: awssdk.String("ch-gva-2"), CreationDate: awssdk.Time(created)},
		{Name: awssdk.String("unrelated-bucket"), BucketRegion: awssdk.String("ch-gva-2"), CreationDate: awssdk.Time(created)},
		{Name: awssdk.String("exasol-f5fd2f97-boostrap"), BucketRegion: awssdk.String("de-fra-1")},
	}
	summaries := map[string]*DeploymentSummary{}

	// When
	addSOSBucketsToSummaries(buckets, "ch-gva-2", "*", summaries)

	// Then only the deployment in the zone is found, dated by its earliest bucket
	if len(summaries) != 1 {
		t.Fatalf("got %d deployments, want 1: %v", len(summaries), summaries)
	}
	sum, ok := summaries["exasol-e59be4f0"]
	if !ok {
		t.Fatalf("deployment exasol-e59be4f0 not discovered: %v", summaries)
	}
	if sum.Resources != 2 {
		t.Errorf("Resources = %d, want 2", sum.Resources)
	}
	if !sum.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want earliest bucket creation %v", sum.CreatedAt, created)
	}
	if sum.Region != "ch-gva-2" {
		t.Errorf("Region = %q, want ch-gva-2", sum.Region)
	}
}

func TestAddSOSBucketsToSummariesRespectsOwnerFilter(t *testing.T) {
	// Given an ownerless bucket and a bucket of a deployment that alice owns
	buckets := []s3types.Bucket{
		{Name: awssdk.String("exasol-e59be4f0-boostrap"), BucketRegion: awssdk.String("ch-gva-2")},
		{Name: awssdk.String("exasol-0badc0de-boostrap"), BucketRegion: awssdk.String("ch-gva-2")},
	}
	summaries := map[string]*DeploymentSummary{
		"exasol-0badc0de": {ID: "exasol-0badc0de", Owner: "alice", Resources: 1},
	}

	// When
	addSOSBucketsToSummaries(buckets, "ch-gva-2", "alice", summaries)

	// Then the ownerless bucket only counts toward a known deployment
	if _, ok := summaries["exasol-e59be4f0"]; ok {
		t.Error("ownerless bucket established a deployment under a specific owner filter")
	}
	if got := summaries["exasol-0badc0de"].Resources; got != 2 {
		t.Errorf("Resources of known deployment = %d, want 2", got)
	}
}

func TestDeploymentIDFromResourceName(t *testing.T) {
	// Given names with and without a deployment id prefix
	cases := map[string]string{
		"exasol-e59be4f0-sos-archive": "exasol-e59be4f0",
		"exasol-e59be4f0-boostrap":    "exasol-e59be4f0",
		"exasol-e59be4f0":             "",
		"exasol-e59be4f01-boostrap":   "",
		"other-e59be4f0-boostrap":     "",
		"audit-trail-e59be4f0":        "",
		"usage-reports-e59be4f0":      "",
	}
	for name, want := range cases {
		// When
		got := deploymentIDFromResourceName(name)

		// Then
		if got != want {
			t.Errorf("deploymentIDFromResourceName(%q) = %q, want %q", name, got, want)
		}
	}
}
