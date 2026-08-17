// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package exoscale

import (
	"context"
	"testing"
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
