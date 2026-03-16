# Azure Infrastructure as Code Architecture

## Overview
This document describes the Infrastructure as Code (IaC) implementation for Exasol Personal on Azure. It supports single-node and multi-node deployments with a simple setup for networking, storage, and unattended installation on Ubuntu virtual machines.

## Prerequisites and Azure Provider
- Authentication is handled by the `azurerm` provider's normal credential chain, such as Azure CLI login or service principal environment variables.
- The provider configuration intentionally stays minimal and relies on the environment for subscription and authentication context.

## Infrastructure Components

### Compute
- One Azure Linux virtual machine is created per node (`n11`, `n12`, ...).
- The default VM size is `Standard_E4s_v3`.
- The default image is Canonical Ubuntu 22.04 LTS Gen2 (`latest`).
- SSH access is configured with a generated RSA key pair and password authentication is disabled.

### Storage
- Each node gets a separate OS disk and a separate managed data disk.
- - Both disks use the Azure managed disk SKU configured by `disk_sku` (default: `StandardSSD_LRS`).
- The data disk is attached at LUN `0`.
- The node metadata exposes a provider-neutral final disk alias `/dev/exasol_data_01`.
- During node preparation, the installer resolves the Azure disk from its LUN and creates the `/dev/exasol_data_01` alias via `udev`, so Exasol can use the same final disk path across providers.
- Remote archive integration is not currently implemented in this Azure preset.

### Networking
- A single resource group contains the deployment resources.
- A single virtual network and subnet are created for the cluster.
- Each node gets:
  - one NIC
  - one static private IP
  - one static Standard public IP
- A network security group is attached to each NIC.

## Access and Security
The following inbound ports are opened from `var.allowed_cidr`:

1. 22 - SSH
2. 2581 - BucketFS
3. 8443 - Admin UI
4. 8563 - Database
5. 20002 - Exasol container SSH
6. 20003 - Exasol confd API

## Resource Organization
- A unique deployment ID is generated for each deployment, for example `exasol-1a2b3c4d`.
- Resource names are derived from the deployment ID to keep deployments isolated.
- Common tags include:
  - `ManagedBy = opentofu`
  - `Project = exasol-personal`
  - `Deployment = <deployment_id>`
  - `CreatedAt = <timestamp>`

## Node Addressing Scheme
- Nodes are named `n<NN>` starting at `n11`.
- Private IPs are assigned deterministically from the subnet:
  - `n11` -> `172.30.1.11`
  - `n12` -> `172.30.1.12`
  - `n13` -> `172.30.1.13`

## Deployment Lifecycle
1. OpenTofu plan/apply:
   - generates a deployment ID
   - creates the resource group, network, NSG, public IPs, NICs, VMs, and managed data disks
   - renders cloud-init for each node
2. Cloud-init on each node:
   - writes deployment metadata and node metadata under `/etc/exasol_launcher/`
   - installs the launcher scripts and systemd units
   - prepares the Azure data disk and exposes it as `/dev/exasol_data_01`
3. Node initialization:
   - systemd runs the shared preparation and installation workflow
   - Exasol is installed using the common disk alias `/dev/exasol_data_01`
4. Local artifacts:
   - `deployment.json` - deployment summary
   - `secrets.json` - generated credentials
   - `node_access.pem` - SSH private key

## Outputs
- The preset exports deployment metadata and deployment secrets as Terraform outputs.
- It also writes `deployment.json` and `secrets.json` into the deployment directory for launcher consumption.
- SSH connection details and database access URLs are included in `deployment.json`.

## Credentials
- Database and Admin UI passwords are generated unless explicitly provided.
- The generated SSH private key is written locally with mode `0600`.

## Permissions
- The operator identity running OpenTofu needs Azure permissions to manage:
  - resource groups
  - virtual networks and subnets
  - public IPs
  - NICs and NSGs
  - virtual machines
  - managed disks
- For broad access, Azure built-in `Contributor` scoped to the target resource group is usually sufficient.
- For least privilege, use a custom Azure RBAC role that covers the resource types above.
- The checked-in policy examples in this directory should be treated as Azure RBAC role-definition examples for this preset.
- Recommended usage:
  - `assets/infrastructure/azure/iam-policy.broad.json` for a broad custom role, or Azure built-in `Contributor`
  - `assets/infrastructure/azure/iam-policy.minimal.json` for a least-privilege custom role scoped to the target resource group or subscription segment
- Assign these permissions at the smallest practical scope, preferably the resource group created for the deployment.

## Configuration Notes
- `power_state` is currently informational only in this Azure preset.
- `s3_archive_enabled` is currently not implemented for Azure and should be treated as reserved.
- `availabilityZone` is currently empty in the generated deployment metadata.
- `disk_sku` controls the Azure managed disk SKU used for both OS and data disks.

## Notes and Limitations
- The preset currently relies on public IP connectivity for node access.
- Archive storage integration is not yet implemented with Azure-native services.
- The SSH private key is written as `node_access.pem`.
