# exoscale-cloud-deployment Specification

## Purpose
Deploy Exasol clusters on Exoscale with VPC networking, and keep deployments created with a Private Network manageable.

## Requirements
### Requirement: Exoscale clusters communicate over a VPC
`exasol install exoscale` SHALL create a VPC with one subnet for each deployment and attach every node to that subnet with a fixed private IPv4 address.

#### Scenario: Install a single-node cluster
- **WHEN** a user whose Exoscale API key may manage compute, networking, SOS, and IAM resources runs `exasol install exoscale`
- **THEN** the installation succeeds
- **AND** `exasol connect -c "SELECT 1"` returns a result

#### Scenario: Install a multi-node cluster
- **WHEN** a user runs `exasol install exoscale --cluster-size 2`
- **THEN** both nodes are attached to the deployment's VPC subnet with distinct private addresses
- **AND** the database runs on both nodes

#### Scenario: Install without access to the networking service
- **WHEN** the IAM role of the Exoscale API key does not allow the `networking` service
- **THEN** the installation fails with an error that contains `Forbidden by role policy for networking`

### Requirement: Destroying an Exoscale deployment removes its VPC
`exasol destroy` SHALL remove the VPC and subnet of an Exoscale deployment together with its other resources.

#### Scenario: Destroy a VPC deployment
- **WHEN** a user runs `exasol destroy` on an Exoscale deployment created with VPC networking
- **THEN** the deployment's VPC and subnet no longer exist in the zone

### Requirement: Exoscale deployments created with a Private Network remain manageable
`exasol stop`, `exasol start`, and `exasol destroy` SHALL operate on an Exoscale deployment whose directory was initialized with Private Network networking.

#### Scenario: Destroy a Private Network deployment
- **WHEN** a user runs `exasol destroy` on an Exoscale deployment initialized before VPC support
- **THEN** the deployment's instances and its Private Network no longer exist in the zone

