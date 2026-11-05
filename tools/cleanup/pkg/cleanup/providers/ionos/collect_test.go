// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package ionos

import (
	"testing"
	"time"
)

var (
	created = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	later   = created.Add(time.Hour)
)

func sampleInventory() inventory {
	return inventory{
		datacenters: []datacenterInventory{
			{
				datacenter: resource{kind: ResourceDatacenter, id: "dc-own", name: "exasol-aaaaaaaa", location: "de/fra", createdAt: later},
				children: []resource{
					{kind: ResourceServer, id: "srv-1", name: "exasol-aaaaaaaa-n11", location: "de/fra", parentID: "dc-own"},
					{kind: ResourceVolume, id: "vol-1", name: "exasol-aaaaaaaa-n11-data", location: "de/fra", parentID: "dc-own"},
					{kind: ResourceLAN, id: "1", name: "exasol-aaaaaaaa-lan", location: "de/fra", parentID: "dc-own"},
				},
			},
			{
				datacenter: resource{kind: ResourceDatacenter, id: "dc-shared", name: "team-sandbox", location: "de/txl"},
				children: []resource{
					{kind: ResourceServer, id: "srv-2", name: "exasol-bbbbbbbb-n11", location: "de/txl", parentID: "dc-shared"},
					{kind: ResourceVolume, id: "vol-2", name: "exasol-bbbbbbbb-n11-os", location: "de/txl", parentID: "dc-shared"},
					{kind: ResourceServer, id: "srv-other", name: "jenkins", location: "de/txl", parentID: "dc-shared"},
				},
			},
		},
		ipBlocks: []resource{
			{kind: ResourceIPBlock, id: "ip-a", name: "exasol-aaaaaaaa-ips", location: "de/fra", createdAt: created},
			{kind: ResourceIPBlock, id: "ip-b", name: "exasol-bbbbbbbb-ips", location: "de/txl"},
			{kind: ResourceIPBlock, id: "ip-other", name: "office", location: "de/fra"},
		},
	}
}

func TestGroupDeploymentsAssignsOwnedDatacenterContents(t *testing.T) {
	t.Parallel()

	// Given a datacenter named after a deployment
	// When resources are grouped into deployments
	deployments := groupDeployments(sampleInventory(), DefaultLocation)

	// Then the deployment owns the datacenter, its contents and its IP block
	owned, ok := deployments["exasol-aaaaaaaa"]
	if !ok {
		t.Fatalf("deployments = %v, want exasol-aaaaaaaa", deployments)
	}
	if owned.Summary.Resources != 5 {
		t.Fatalf("resources = %d, want datacenter, server, volume, LAN and IP block", owned.Summary.Resources)
	}
	if !owned.Summary.CreatedAt.Equal(created) {
		t.Fatalf("createdAt = %v, want earliest resource time %v", owned.Summary.CreatedAt, created)
	}
	if owned.Summary.Region != "de/fra" || owned.Summary.Owner != "-" || owned.Summary.State != "active" {
		t.Fatalf("summary = %+v", owned.Summary)
	}
}

func TestGroupDeploymentsPicksPrefixedResourcesFromForeignDatacenter(t *testing.T) {
	t.Parallel()

	// Given deployment resources inside a datacenter the launcher did not create
	// When resources are grouped into deployments
	deployments := groupDeployments(sampleInventory(), DefaultLocation)

	// Then only the prefixed resources belong to the deployment, not the datacenter
	if len(deployments) != 2 {
		t.Fatalf("deployments = %d, want 2", len(deployments))
	}
	foreign := deployments["exasol-bbbbbbbb"]
	for _, res := range foreign.Resources {
		if res.Ref.Type == ResourceDatacenter || res.Ref.ID == "srv-other" {
			t.Fatalf("foreign deployment includes %+v", res.Ref)
		}
	}
	if foreign.Summary.Resources != 3 {
		t.Fatalf("resources = %d, want server, volume and IP block", foreign.Summary.Resources)
	}
}

func TestGroupDeploymentsKeepsLauncherDatacenterHostingAnotherDeployment(t *testing.T) {
	t.Parallel()

	// Given a second deployment installed into the first deployment's datacenter
	inv := inventory{datacenters: []datacenterInventory{{
		datacenter: resource{kind: ResourceDatacenter, id: "dc-a", name: "exasol-aaaaaaaa", location: "de/fra"},
		children: []resource{
			{kind: ResourceServer, id: "srv-a", name: "exasol-aaaaaaaa-n11", location: "de/fra", parentID: "dc-a"},
			{kind: ResourceServer, id: "srv-b", name: "exasol-bbbbbbbb-n11", location: "de/fra", parentID: "dc-a"},
			{kind: ResourceLAN, id: "2", name: "exasol-bbbbbbbb-lan", location: "de/fra", parentID: "dc-a"},
		},
	}}}

	// When resources are grouped into deployments
	deployments := groupDeployments(inv, DefaultLocation)

	// Then each deployment gets only its own resources and neither owns the datacenter
	first, second := deployments["exasol-aaaaaaaa"], deployments["exasol-bbbbbbbb"]
	if first == nil || second == nil {
		t.Fatalf("deployments = %v, want exasol-aaaaaaaa and exasol-bbbbbbbb", deployments)
	}
	if len(first.Resources) != 1 || first.Resources[0].Ref.ID != "srv-a" {
		t.Fatalf("first deployment resources = %+v, want only srv-a", first.Resources)
	}
	if len(second.Resources) != 2 {
		t.Fatalf("second deployment resources = %+v, want server and LAN", second.Resources)
	}
	actions, err := PlanActions(first, nil)
	if err != nil {
		t.Fatalf("PlanActions returned error: %v", err)
	}
	assertRefs(t, actions, "delete:srv-a")
}

func TestGroupDeploymentsFiltersByLocation(t *testing.T) {
	t.Parallel()

	// Given deployments in two locations
	// When resources are grouped for one location
	deployments := groupDeployments(sampleInventory(), "de/txl")

	// Then only the deployment in that location is returned
	if len(deployments) != 1 {
		t.Fatalf("deployments = %d, want 1", len(deployments))
	}
	if _, ok := deployments["exasol-bbbbbbbb"]; !ok {
		t.Fatalf("deployments = %v, want exasol-bbbbbbbb", deployments)
	}
}

func TestGroupDeploymentsReportsLeftoverIPBlockAsOrphaned(t *testing.T) {
	t.Parallel()

	// Given an IP block left over without its datacenter
	inv := inventory{ipBlocks: []resource{{kind: ResourceIPBlock, id: "ip-c", name: "exasol-cccccccc-ips", location: "de/fra"}}}

	// When resources are grouped into deployments
	details := groupDeployments(inv, DefaultLocation)["exasol-cccccccc"]

	// Then the deployment is reported as orphaned
	if details == nil || details.Summary.State != "orphaned" {
		t.Fatalf("details = %+v, want orphaned deployment", details)
	}
}

func TestDeploymentFromName(t *testing.T) {
	t.Parallel()

	// Given resource names with and without a deployment prefix
	cases := map[string]string{
		"exasol-0123abcd-n11-data": "exasol-0123abcd",
		"exasol-0123abcd-ips":      "exasol-0123abcd",
		"exasol-0123abcd":          "",
		"exasol-XYZ-ips":           "",
		"myexasol-0123abcd-ips":    "",
	}
	for name, want := range cases {
		// When the deployment id is derived from the name
		got, ok := deploymentFromName(name)
		// Then only prefixed child resource names yield it
		if got != want || ok != (want != "") {
			t.Fatalf("deploymentFromName(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestListAllFetchesEveryPage(t *testing.T) {
	t.Parallel()

	// Given a listing that spans more than one page
	total := int(pageSize) + 5
	var offsets []int32
	fetch := func(offset, limit int32) (*[]int, error) {
		offsets = append(offsets, offset)
		page := []int{}
		for i := int(offset); i < total && i < int(offset+limit); i++ {
			page = append(page, i)
		}

		return &page, nil
	}

	// When every item is listed
	items, err := listAll(fetch)
	if err != nil {
		t.Fatalf("listAll returned error: %v", err)
	}

	// Then all items are returned and listing stops after the short page
	if len(items) != total {
		t.Fatalf("items = %d, want %d", len(items), total)
	}
	if len(offsets) != 2 || offsets[1] != pageSize {
		t.Fatalf("offsets = %v, want [0 %d]", offsets, pageSize)
	}
}

func TestOwnerFilterMatchesUnowned(t *testing.T) {
	t.Parallel()

	// Given owner filters, including ones that match the "-" display placeholder
	cases := map[string]bool{"": true, "*": true, "-": false, "*-*": false, "alice": false}
	for filter, want := range cases {
		// When an IONOS deployment, which has no owner, is matched against the filter
		got := ownerFilterMatchesUnowned(filter)
		// Then only filters that select any owner match it
		if got != want {
			t.Fatalf("ownerFilterMatchesUnowned(%q) = %v, want %v", filter, got, want)
		}
	}
}
