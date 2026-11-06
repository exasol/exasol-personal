// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	ionoscloud "github.com/ionos-cloud/sdk-go/v6"
)

const listDepth = 1

var errMissingCredentials = errors.New(
	"IONOS credentials not set; set IONOS_TOKEN (or IONOS_USERNAME and IONOS_PASSWORD)")

func hasCredentials() bool {
	if os.Getenv(ionoscloud.IonosTokenEnvVar) != "" {
		return true
	}

	return os.Getenv(ionoscloud.IonosUsernameEnvVar) != "" && os.Getenv(ionoscloud.IonosPasswordEnvVar) != ""
}

// newClient builds an IONOS Cloud API client from the same environment
// variables the IONOS Terraform provider reads.
func newClient() (*ionoscloud.APIClient, error) {
	if !hasCredentials() {
		return nil, errMissingCredentials
	}

	return ionoscloud.NewAPIClient(ionoscloud.NewConfigurationFromEnv()), nil
}

// GetAccountInfo verifies connectivity by listing one datacenter and returns a
// human-readable account string.
func GetAccountInfo(ctx context.Context) (string, error) {
	api, err := newClient()
	if err != nil {
		return "", err
	}

	if _, _, err := api.DataCentersApi.DatacentersGet(ctx).Depth(0).Limit(1).Execute(); err != nil {
		return "", fmt.Errorf("failed to reach IONOS Cloud API: %w", err)
	}

	if user := os.Getenv(ionoscloud.IonosUsernameEnvVar); user != "" {
		return "user " + user, nil
	}

	return "token", nil
}

// listInventory lists the datacenters, their servers, volumes and LANs, and the
// IP blocks in the contract. Datacenters outside the location filter are not
// descended into.
func listInventory(ctx context.Context, api *ionoscloud.APIClient, location string) (inventory, error) {
	var inv inventory

	datacenters, err := listAll(func(offset, limit int32) (*[]ionoscloud.Datacenter, error) {
		page, _, err := api.DataCentersApi.DatacentersGet(ctx).
			Depth(listDepth).Offset(offset).Limit(limit).Execute()

		return page.Items, err
	})
	if err != nil {
		return inv, fmt.Errorf("failed to list IONOS datacenters: %w", err)
	}
	for _, dc := range datacenters {
		dcRes := resource{
			kind:      ResourceDatacenter,
			id:        str(dc.GetId()),
			name:      str(dc.GetProperties().GetName()),
			location:  str(dc.GetProperties().GetLocation()),
			createdAt: timeOf(dc.GetMetadata().GetCreatedDate()),
			state:     str(dc.GetMetadata().GetState()),
		}
		if !matchesLocation(location, dcRes.location) {
			continue
		}

		children, err := listDatacenterChildren(ctx, api, dcRes)
		if err != nil {
			return inv, err
		}
		inv.datacenters = append(inv.datacenters, datacenterInventory{datacenter: dcRes, children: children})
	}

	blocks, err := listAll(func(offset, limit int32) (*[]ionoscloud.IpBlock, error) {
		page, _, err := api.IPBlocksApi.IpblocksGet(ctx).
			Depth(listDepth).Offset(offset).Limit(limit).Execute()

		return page.Items, err
	})
	if err != nil {
		return inv, fmt.Errorf("failed to list IONOS IP blocks: %w", err)
	}
	for _, block := range blocks {
		inv.ipBlocks = append(inv.ipBlocks, resource{
			kind:      ResourceIPBlock,
			id:        str(block.GetId()),
			name:      str(block.GetProperties().GetName()),
			location:  str(block.GetProperties().GetLocation()),
			createdAt: timeOf(block.GetMetadata().GetCreatedDate()),
			state:     str(block.GetMetadata().GetState()),
		})
	}

	return inv, nil
}

func listDatacenterChildren(ctx context.Context, api *ionoscloud.APIClient, dc resource) ([]resource, error) {
	child := func(kind ResourceType, id, name *string, metadata *ionoscloud.DatacenterElementMetadata, state *string) resource {
		return resource{
			kind:      kind,
			id:        str(id),
			name:      str(name),
			location:  dc.location,
			parentID:  dc.id,
			createdAt: timeOf(metadata.GetCreatedDate()),
			state:     str(state),
		}
	}

	var children []resource

	servers, err := listAll(func(offset, limit int32) (*[]ionoscloud.Server, error) {
		page, _, err := api.ServersApi.DatacentersServersGet(ctx, dc.id).
			Depth(listDepth).Offset(offset).Limit(limit).Execute()

		return page.Items, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list servers in IONOS datacenter %s: %w", dc.id, err)
	}
	for _, server := range servers {
		children = append(children, child(ResourceServer, server.GetId(),
			server.GetProperties().GetName(), server.GetMetadata(), server.GetProperties().GetVmState()))
	}

	volumes, err := listAll(func(offset, limit int32) (*[]ionoscloud.Volume, error) {
		page, _, err := api.VolumesApi.DatacentersVolumesGet(ctx, dc.id).
			Depth(listDepth).Offset(offset).Limit(limit).Execute()

		return page.Items, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes in IONOS datacenter %s: %w", dc.id, err)
	}
	for _, volume := range volumes {
		children = append(children, child(ResourceVolume, volume.GetId(),
			volume.GetProperties().GetName(), volume.GetMetadata(), volume.GetMetadata().GetState()))
	}

	lans, err := listAll(func(offset, limit int32) (*[]ionoscloud.Lan, error) {
		page, _, err := api.LANsApi.DatacentersLansGet(ctx, dc.id).
			Depth(listDepth).Offset(offset).Limit(limit).Execute()

		return page.Items, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list LANs in IONOS datacenter %s: %w", dc.id, err)
	}
	for _, lan := range lans {
		children = append(children, child(ResourceLAN, lan.GetId(),
			lan.GetProperties().GetName(), lan.GetMetadata(), lan.GetMetadata().GetState()))
	}

	return children, nil
}

// deleteResource deletes one resource and waits for IONOS to finish the
// request, so dependent deletions that follow see it gone. A resource that no
// longer exists counts as deleted.
func deleteResource(ctx context.Context, api *ionoscloud.APIClient, ref ResourceRef) error {
	var (
		resp *ionoscloud.APIResponse
		err  error
	)
	switch ref.Type {
	case ResourceDatacenter:
		resp, err = api.DataCentersApi.DatacentersDelete(ctx, ref.ID).Execute()
	case ResourceServer:
		resp, err = api.ServersApi.DatacentersServersDelete(ctx, ref.ParentID, ref.ID).Execute()
	case ResourceVolume:
		resp, err = api.VolumesApi.DatacentersVolumesDelete(ctx, ref.ParentID, ref.ID).Execute()
	case ResourceLAN:
		resp, err = api.LANsApi.DatacentersLansDelete(ctx, ref.ParentID, ref.ID).Execute()
	case ResourceIPBlock:
		resp, err = api.IPBlocksApi.IpblocksDelete(ctx, ref.ID).Execute()
	default:
		return fmt.Errorf("unsupported IONOS resource type %s", ref.Type)
	}
	if isNotFound(resp) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete %s %s: %w", ref.Type, ref.ID, err)
	}

	requestPath := resp.Header.Get("Location")
	if requestPath == "" {
		return nil
	}
	if _, err := api.WaitForRequest(ctx, requestPath); err != nil {
		return fmt.Errorf("deletion of %s %s did not complete: %w", ref.Type, ref.ID, err)
	}

	return nil
}

func isNotFound(resp *ionoscloud.APIResponse) bool {
	return resp != nil && resp.Response != nil && resp.StatusCode == http.StatusNotFound
}

// ownerMatchesFilter reports whether an owner matches the filter, supporting a
// simple '*' wildcard. An empty or '*' filter matches everything.
// ownerFilterMatchesUnowned reports whether an owner filter selects IONOS
// deployments, whose resources carry no owner. The "-" shown as their owner is
// only a display placeholder and must not be matched.
func ownerFilterMatchesUnowned(filter string) bool {
	return ownerMatchesFilter("", filter)
}

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

func listAll[T any](fetch func(offset, limit int32) (*[]T, error)) ([]T, error) {
	var all []T
	for offset := int32(0); ; offset += pageSize {
		items, err := fetch(offset, pageSize)
		if err != nil {
			return nil, err
		}
		page := itemsOf(items)
		all = append(all, page...)
		if len(page) < int(pageSize) {
			return all, nil
		}
	}
}

func itemsOf[T any](items *[]T) []T {
	if items == nil {
		return nil
	}

	return *items
}

func str(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func timeOf(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}

	return *value
}
