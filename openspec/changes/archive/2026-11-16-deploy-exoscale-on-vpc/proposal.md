## Why

Every new Exoscale deployment fails during infrastructure creation with `Operation 'create-private-network' not enabled`, because Exoscale no longer allows Private Networks for new workloads and recommends VPC instead. The nightly cleanup of orphaned deployment-test resources has also never removed an Exoscale deployment whose instances, volumes, and private network were already gone, so buckets and other unlabeled leftovers accumulate.

## What Changes

- New Exoscale deployments connect their nodes through a per-deployment VPC and subnet with fixed private IPv4 addresses instead of a Private Network.
- The Exoscale IAM example policies grant the `networking` service that VPC management requires; this is a breaking change for API keys whose role lacks it.
- Existing Private Network deployments keep the preset they were created with and remain manageable.
- The cleanup tool discovers Exoscale deployments from any resource named after the deployment, including VPCs and subnets, and deletes VPC resources after the instances that use them.
- The changelog and the Exoscale user documentation describe the change.

## Capabilities

### New Capabilities

- `exoscale-cloud-deployment`: Exoscale cluster networking over VPC and lifecycle of deployments created with a Private Network.
- `orphaned-deployment-cleanup`: Discovery and removal of leftover Exoscale deployment-test resources by the cleanup tool.

### Modified Capabilities

None.

## Impact

SPOT-34037 affects the Exoscale infrastructure preset, its provider version and IAM example policies, the Exoscale cleanup provider, the changelog, and the Exoscale user documentation. Migrating existing deployments from Private Networks to VPC is out of scope.
