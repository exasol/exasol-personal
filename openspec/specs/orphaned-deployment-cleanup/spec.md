# orphaned-deployment-cleanup Specification

## Purpose
Discover and remove leftover deployment-test resources, whichever of a deployment's resources remain.

## Requirements
### Requirement: The cleanup tool discovers leftover deployments from any of their resources
`exasol-cleanup discover --owner=*` SHALL report a deployment of any supported provider when any resource that the provider's API exposes with the deployment's id in its name, tag, label, or containing resource group remains in a searched location. For Exoscale these are compute instances, block storage volumes, private networks, VPCs, VPC subnets, security groups, SSH keys, IAM roles, IAM API keys, and SOS buckets.

#### Scenario: Only a bootstrap bucket remains
- **WHEN** only the bucket `exasol-<id>-boostrap` of a deployment remains in the zone
- **THEN** `discover --owner=*` lists deployment `exasol-<id>` with its bucket's creation time

#### Scenario: Only unlabeled compute and IAM resources remain
- **WHEN** only the security group, SSH key, IAM role, and IAM API key of a deployment remain
- **THEN** `discover --owner=*` lists the deployment with four resources

#### Scenario: Only VPC resources remain
- **WHEN** only the VPC and subnet of a deployment remain
- **THEN** `discover --owner=*` lists the deployment with two resources

#### Scenario: Ownerless resources count toward the requested owner's deployments
- **WHEN** the owner filter names a specific owner whose deployment also has an SOS bucket
- **THEN** `discover` lists only that owner's deployments, with the bucket counted in its resources

#### Scenario: Deployments are identified by their name prefix
- **WHEN** the zone holds the bucket `exasol-<id>-boostrap` next to organization buckets such as `audit-trail-<suffix>` and `usage-reports-<suffix>`
- **THEN** `discover --owner=*` lists exactly the deployment `exasol-<id>`

#### Scenario: Organization-wide resources of a deployment in another zone
- **WHEN** a deployment's security group and SSH key exist and its instances run in another zone
- **THEN** `discover --owner=*` lists that deployment only in the zone of its instances

#### Scenario: Only organization-wide resources remain and several zones are searched
- **WHEN** only a deployment's security group and SSH key remain and `--exoscale-zone=ch-gva-2,de-fra-1` is searched
- **THEN** `discover --owner=*` lists the deployment once
- **AND** `run` resolves the deployment to one zone instead of failing with `deployment_ambiguous`

### Requirement: Age-filtered cleanup leaves undated organization-wide leftovers alone
`exasol-cleanup run --all --older-than=<age>` and `discover --older-than=<age>` SHALL exclude a deployment that only resources without a creation time and without a location reveal, such as Exoscale security groups, SSH keys, and IAM roles.

#### Scenario: Only a security group and SSH key remain
- **WHEN** only the security group and SSH key of an Exoscale deployment remain
- **THEN** `run --all --older-than=24h --owner=*` does not select the deployment
- **AND** `discover --owner=*` without `--older-than` lists it, and `run <id> --execute` removes it

### Requirement: The cleanup tool removes Exoscale VPC resources after their users
`exasol-cleanup run --execute` SHALL delete a deployment's compute instances before its VPC subnets, and its VPC subnets before its VPC.

#### Scenario: Plan a deployment with instances and VPC resources
- **WHEN** a deployment has compute instances, a VPC subnet, and a VPC
- **THEN** the planned actions delete the instances first, then the subnet, then the VPC

