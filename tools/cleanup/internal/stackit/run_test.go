// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package stackit

import (
	"context"
	"reflect"
	"testing"
	"time"

	objectstorage "github.com/stackitcloud/stackit-sdk-go/services/objectstorage/v2api"
)

func TestPlanActionsOrdersStackitNetworkDependencies(t *testing.T) {
	t.Parallel()

	// Given a deployment with dependent STACKIT networking resources
	details := &DeploymentDetails{
		Resources: []ResourceMeta{
			{Ref: ResourceRef{Type: ResourceNetwork, ID: "network-1"}},
			{Ref: ResourceRef{Type: ResourceNetworkInterface, ID: "nic-1", ParentID: "network-1"}},
			{Ref: ResourceRef{Type: ResourcePublicIP, ID: "public-ip-1"}},
			{Ref: ResourceRef{Type: ResourceSecurityGroup, ID: "sg-1"}},
			{Ref: ResourceRef{Type: ResourceServer, ID: "server-1"}},
		},
	}

	// When cleanup actions are planned
	actions, err := PlanActions(details, nil)
	if err != nil {
		t.Fatalf("unexpected error planning actions: %v", err)
	}

	// Then public IPs and NICs are deleted before networks and security groups
	got := make([]ResourceType, 0, len(actions))
	for _, action := range actions {
		got = append(got, action.Ref.Type)
	}

	want := []ResourceType{
		ResourceServer,
		ResourcePublicIP,
		ResourceNetworkInterface,
		ResourceNetwork,
		ResourceSecurityGroup,
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d actions, got %d", len(want), len(got))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected action %d to be %q, got %q", index, want[index], got[index])
		}
	}
}

func TestDeleteMatchingCredentialsGroupAccessKeysUsesCredentialsGroupFilter(t *testing.T) {
	t.Parallel()

	// Given a STACKIT handler with objectstorage mocks
	deletedKeyIDs := []string{}
	deletedKeyGroups := []string{}
	listAccessKeysExecute := func(r objectstorage.ApiListAccessKeysRequest) (*objectstorage.ListAccessKeysResponse, error) {
		filter := reflect.ValueOf(r).FieldByName("credentialsGroup")
		if !filter.IsValid() || filter.IsNil() {
			t.Fatal("expected credentials-group filter to be set")
		}
		if got := filter.Elem().String(); got != "group-1" {
			t.Fatalf("expected credentials-group filter %q, got %q", "group-1", got)
		}

		accessKey := objectstorage.AccessKey{}
		accessKey.SetKeyId("key-1")
		return objectstorage.NewListAccessKeysResponse([]objectstorage.AccessKey{accessKey}, "project-1"), nil
	}
	deleteAccessKeyExecute := func(r objectstorage.ApiDeleteAccessKeyRequest) (*objectstorage.DeleteAccessKeyResponse, error) {
		deletedKeyIDs = append(deletedKeyIDs, reflect.ValueOf(r).FieldByName("keyId").String())
		groupField := reflect.ValueOf(r).FieldByName("credentialsGroup")
		if !groupField.IsValid() || groupField.IsNil() {
			t.Fatal("expected credentials-group filter to be set on delete")
		}
		deletedKeyGroups = append(deletedKeyGroups, groupField.Elem().String())
		return objectstorage.NewDeleteAccessKeyResponse("key-1", "project-1"), nil
	}

	h := handler{
		objectStorageClient: &objectstorage.APIClient{DefaultAPI: objectstorage.DefaultAPIServiceMock{
			ListAccessKeysExecuteMock:  &listAccessKeysExecute,
			DeleteAccessKeyExecuteMock: &deleteAccessKeyExecute,
		}},
		projectId: "project-1",
		region:    "eu01",
	}

	// When credential-group access keys are deleted
	deletedCount, err := h.deleteMatchingCredentialsGroupAccessKeys(context.Background(), "group-1")
	if err != nil {
		t.Fatalf("unexpected error deleting access keys: %v", err)
	}

	// Then only the filtered group's keys are deleted
	if deletedCount != 1 {
		t.Fatalf("expected 1 deleted access key, got %d", deletedCount)
	}
	if len(deletedKeyIDs) != 1 || deletedKeyIDs[0] != "key-1" {
		t.Fatalf("expected deleted key IDs [key-1], got %v", deletedKeyIDs)
	}
	if len(deletedKeyGroups) != 1 || deletedKeyGroups[0] != "group-1" {
		t.Fatalf("expected deleted key groups [group-1], got %v", deletedKeyGroups)
	}
}

func TestDeleteObjectStorageCredentialUsesParentCredentialsGroup(t *testing.T) {
	t.Parallel()

	// Given a credential resource that knows its parent credentials group
	var deletedKeyGroup string
	deleteAccessKeyExecute := func(r objectstorage.ApiDeleteAccessKeyRequest) (*objectstorage.DeleteAccessKeyResponse, error) {
		groupField := reflect.ValueOf(r).FieldByName("credentialsGroup")
		if !groupField.IsValid() || groupField.IsNil() {
			t.Fatal("expected credentials-group filter to be set on delete")
		}
		deletedKeyGroup = groupField.Elem().String()
		return objectstorage.NewDeleteAccessKeyResponse("key-1", "project-1"), nil
	}

	h := handler{
		objectStorageClient: &objectstorage.APIClient{DefaultAPI: objectstorage.DefaultAPIServiceMock{
			DeleteAccessKeyExecuteMock: &deleteAccessKeyExecute,
		}},
		projectId: "project-1",
		region:    "eu01",
	}

	// When the direct credential delete runs
	err := h.DeleteObjectStorageCredential(context.Background(), ResourceRef{
		Type:     ResourceObjectStorageCredential,
		ID:       "key-1",
		ParentID: "group-1",
	})
	if err != nil {
		t.Fatalf("unexpected error deleting object storage credential: %v", err)
	}

	// Then the delete request carries the parent credentials group
	if deletedKeyGroup != "group-1" {
		t.Fatalf("expected delete to use credentials group %q, got %q", "group-1", deletedKeyGroup)
	}
}

func TestResourceMetaFromAccessKeyStoresCredentialsGroupAsParent(t *testing.T) {
	t.Parallel()

	// Given an access key returned with group metadata
	accessKey := objectstorage.AccessKey{}
	accessKey.SetDisplayName("key-1")
	accessKey.SetExpires("")
	accessKey.SetKeyId("key-1")
	accessKey.AdditionalProperties = map[string]interface{}{
		"credentialsGroupId": "group-1",
	}

	// When resource metadata is derived from the access key
	meta, err := ResourceMetaFromAccessKey(accessKey, "project-1", "eu01")
	if err != nil {
		t.Fatalf("unexpected error building access-key metadata: %v", err)
	}

	// Then the resource keeps the group relationship for deletion
	if meta.Ref.ParentID != "group-1" {
		t.Fatalf("expected parent credentials group %q, got %q", "group-1", meta.Ref.ParentID)
	}
}

func TestTemporaryObjectStorageCredentialsGroupNameFitsApiLimit(t *testing.T) {
	t.Parallel()

	// Given a deterministic timestamp
	now := time.Unix(0, 1780070569901435109)

	// When a temporary credentials-group name is generated
	name := temporaryObjectStorageCredentialsGroupName(now)

	// Then the generated name is valid for the STACKIT API limit
	if len(name) > stackitTempBucketCredsMaxNameLength {
		t.Fatalf("expected name length <= %d, got %d (%q)", stackitTempBucketCredsMaxNameLength, len(name), name)
	}
	if name == "" {
		t.Fatal("expected non-empty temporary credentials-group name")
	}
	if got, want := name[:len(stackitTempBucketCredsPrefix)], stackitTempBucketCredsPrefix; got != want {
		t.Fatalf("expected name prefix %q, got %q", want, got)
	}
}
