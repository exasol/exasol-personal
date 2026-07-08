// Copyright 2026 Exasol AG
// SPDX-License-Identifier: MIT

package azure

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// clients bundles the Azure Resource Manager clients used for discovery and cleanup.
type clients struct {
	groups    *armresources.ResourceGroupsClient
	resources *armresources.Client
}

// newCredential builds the Azure credential chain the tool authenticates with.
// It supports the same credential types as DefaultAzureCredential — service
// principal / OIDC via environment, workload identity, the Azure CLI, the Azure
// Developer CLI, Azure PowerShell, and managed identity — but is assembled
// explicitly so managed identity is tried *after* the developer/operator logins.
//
// DefaultAzureCredential probes managed identity before the CLI and aborts the
// whole chain on the first hard (non-"unavailable") error. On an Azure Arc
// enrolled host the managed-identity IMDS probe hard-fails reading the root-only
// Arc token key, which would otherwise mask a perfectly good `az login`. Ordering
// managed identity last keeps every authentication method available while making
// an operator's interactive login reliably usable, with no configuration needed.
func newCredential() (azcore.TokenCredential, error) {
	var sources []azcore.TokenCredential

	// Service principal (client secret/certificate) or OIDC via environment
	// variables — the usual CI/automation path; only present when configured.
	if cred, err := azidentity.NewEnvironmentCredential(nil); err == nil {
		sources = append(sources, cred)
	}
	// Federated workload identity (e.g. AKS); only present when configured.
	if cred, err := azidentity.NewWorkloadIdentityCredential(nil); err == nil {
		sources = append(sources, cred)
	}
	// Developer / operator logins.
	if cred, err := azidentity.NewAzureCLICredential(nil); err == nil {
		sources = append(sources, cred)
	}
	if cred, err := azidentity.NewAzureDeveloperCLICredential(nil); err == nil {
		sources = append(sources, cred)
	}
	if cred, err := azidentity.NewAzurePowerShellCredential(nil); err == nil {
		sources = append(sources, cred)
	}
	// Managed identity last: its IMDS probe can hard-fail on Arc-enrolled hosts
	// and must not preempt a working developer credential above.
	if cred, err := azidentity.NewManagedIdentityCredential(nil); err == nil {
		sources = append(sources, cred)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no Azure credential available; run `az login` or set service-principal environment variables")
	}

	return azidentity.NewChainedTokenCredential(sources, nil)
}

// newClients builds the Azure clients for a subscription using the credential
// chain from newCredential.
func newClients(subscriptionID string) (*clients, error) {
	if subscriptionID == "" {
		return nil, fmt.Errorf("azure subscription id is required")
	}

	cred, err := newCredential()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain Azure credential: %w", err)
	}

	groups, err := armresources.NewResourceGroupsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure resource groups client: %w", err)
	}

	resources, err := armresources.NewClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure resources client: %w", err)
	}

	return &clients{groups: groups, resources: resources}, nil
}

// GetAccountInfo verifies connectivity for the subscription by listing the
// first page of resource groups and returns a human-readable account string.
func GetAccountInfo(ctx context.Context, subscriptionID string) (string, error) {
	cl, err := newClients(subscriptionID)
	if err != nil {
		return "", err
	}

	pager := cl.groups.NewListPager(nil)
	if pager.More() {
		if _, err := pager.NextPage(ctx); err != nil {
			return "", fmt.Errorf("failed to reach Azure subscription %s: %w", subscriptionID, err)
		}
	}

	return fmt.Sprintf("subscription %s", subscriptionID), nil
}

// deleteResourceGroup deletes a resource group and waits for the operation to
// complete. Azure cascades the deletion to every resource the group contains.
func deleteResourceGroup(ctx context.Context, subscriptionID, resourceGroup string) error {
	cl, err := newClients(subscriptionID)
	if err != nil {
		return err
	}

	poller, err := cl.groups.BeginDelete(ctx, resourceGroup, nil)
	if err != nil {
		return fmt.Errorf("failed to start deletion of resource group %s: %w", resourceGroup, err)
	}

	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("failed to delete resource group %s: %w", resourceGroup, err)
	}

	return nil
}

// classifyAzureType maps an Azure resource type string (e.g.
// "Microsoft.Compute/virtualMachines") to a cleanup ResourceType.
func classifyAzureType(azureType string) ResourceType {
	switch strings.ToLower(azureType) {
	case "microsoft.compute/virtualmachines":
		return ResourceVirtualMachine
	case "microsoft.compute/disks":
		return ResourceDisk
	case "microsoft.network/networkinterfaces":
		return ResourceNetworkIface
	case "microsoft.network/publicipaddresses":
		return ResourcePublicIP
	case "microsoft.network/virtualnetworks":
		return ResourceVirtualNetwork
	case "microsoft.network/networksecuritygroups":
		return ResourceSecurityGroup
	case "microsoft.storage/storageaccounts":
		return ResourceStorageAccount
	default:
		return ResourceGeneric
	}
}

// tagValue returns the value of a tag, or empty string when absent.
func tagValue(tags map[string]*string, key string) string {
	if value, ok := tags[key]; ok && value != nil {
		return *value
	}

	return ""
}

// tagsToMap converts Azure's map[string]*string tags to a plain map.
func tagsToMap(tags map[string]*string) map[string]string {
	mapped := make(map[string]string, len(tags))
	for key, value := range tags {
		if value != nil {
			mapped[key] = *value
		}
	}

	return mapped
}

// parseCreatedAt parses an RFC3339 CreatedAt tag value; the zero time is
// returned when the tag is missing or malformed.
func parseCreatedAt(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts
	}

	return time.Time{}
}

// ownerMatchesFilter reports whether an owner matches the filter, supporting a
// simple '*' wildcard. An empty or '*' filter matches everything.
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

// matchesLocation reports whether a resource group in rgLocation should be
// included for a collector scoped to collectorLocation. The "all" sentinel (or
// an empty location) matches every region.
func matchesLocation(collectorLocation, rgLocation string) bool {
	if collectorLocation == "" || collectorLocation == DefaultLocation {
		return true
	}

	return strings.EqualFold(collectorLocation, rgLocation)
}

// matchesDeploymentTags reports whether a resource group's tags identify an
// Exasol Personal deployment. In legacy mode only the Deployment tag format is
// required; otherwise Project=exasol-personal must also be present.
func matchesDeploymentTags(tags map[string]*string, legacy bool) bool {
	deployment := tagValue(tags, tagDeployment)
	if !deploymentIDRegex.MatchString(deployment) {
		return false
	}
	if legacy {
		return true
	}

	return tagValue(tags, tagProject) == projectValue
}
