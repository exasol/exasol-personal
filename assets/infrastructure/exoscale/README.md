# Exoscale Infrastructure as Code Architecture

## Overview
This document describes the Infrastructure as Code (IaC) implementation for Exasol Personal on Exoscale. It supports both single-node and multi-node (cluster) deployments with a simple, opinionated setup for networking, storage, and installation.

## Prerequisites and Exoscale Provider
- Exoscale API credentials are taken from environment variables: `EXOSCALE_API_KEY` and `EXOSCALE_API_SECRET`.
- Zone selection is required via `TF_VAR_zone` (e.g., `ch-gva-2`, `de-fra-1`, `at-vie-1`). Unlike AWS availability zones, Exoscale zones are analogous to AWS regions.
- For SOS (S3-compatible object storage) access, the same credentials can be mapped to `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` for the `aws.sos` provider.
- The provider configuration in `providers.tf` uses these environment variables and does not define credentials inline.

## Infrastructure Components

### Compute
- Exoscale compute instances named after their node IDs (e.g., `n11`, `n12`, ...).
- Default instance type `standard.extra-large` (4 vCPU / 32 GB RAM; configurable via `var.instance_type`).
- Ubuntu template: uses `Linux Ubuntu 22.04 LTS 64-bit` from Exoscale's public template catalog.
- Cluster support: one instance per node; controlled by `var.cluster_size` (default: 1).
- Instances get public IPv4 addresses automatically; no Elastic IP management required.

### Storage
- Separate volumes for OS/root and database data.
- OS disk size configurable via `var.os_volume_size` (minimum 10 GB).
- Data volumes are separate block storage volumes, encrypted by default, attached via `block_storage_volume_ids`.
- Data volume size configurable via `var.data_volume_size` (in GB).
- A remote archive volume on Exoscale SOS (S3-compatible object storage) is created and registered automatically if `var.s3_archive_enabled` is true (default).
  - A per-deployment SOS bucket is created using a globally unique name.
  - Access to the bucket is granted via an IAM role and API key pair delivered to instances via cloud-init (no instance profiles; explicit credentials).

### Networking
- **Simplified Model**: Exoscale instances receive public IP addresses automatically. No VPC, Internet Gateway, or route tables are needed.
- A managed private network is created for inter-node communication with static DHCP leases.
- Private IPs are assigned from the `172.30.1.0/24` range.
- Security groups control inbound/outbound traffic. Exoscale security groups are stateful and use separate rule resources.
- **No VPC Endpoints**: SOS access goes directly over HTTPS to `sos-<zone>.exo.io`; no private network isolation for object storage.

## Access and Security
The following ports are exposed via security group rules (configurable per port):

1. 22 — SSH access (public)
2. 2581 — Default bucketfs (public)
3. 8443 — Admin UI HTTPS (public)
4. 8563 — Default database port (public)
5. 20002 — Exasol container SSH (public)
6. 20003 — Exasol confd API (public)

Additionally, internal traffic (TCP/UDP/ICMP) is allowed between cluster nodes via a self-referencing security group rule.

## Resource Organization (Labels)
- A unique deployment ID is generated at apply time (e.g., `exasol-<deployment_id>`).
- Labels (key/value pairs) are applied to resources that support them:
  - Compute instances
  - Private networks
  - Block storage volumes
- Labels include:
  - `name = <resource_name>`
  - `deployment_id = <deployment_id>`
- Not all resources support labels (e.g., security groups, SSH keys, IAM roles).

## Node Addressing Scheme
- Nodes are identified as `n<NN>` starting at `n11`.
- Private IPs are statically assigned via the private network: `172.30.1.<11 + i>` where `i` ranges from `0` to `var.cluster_size - 1`.
- Example for `cluster_size = 3`:
  - `n11` → `172.30.1.11`
  - `n12` → `172.30.1.12`
  - `n13` → `172.30.1.13`
- These addresses are passed to the installer for cluster configuration.

## Deployment Lifecycle
1. Terraform plan/apply:
   - Generates a deployment ID and a new ECDSA SSH key pair.
   - Provisions the private network, security group/rules, compute instances, and block storage volumes.
   - Registers the SSH public key with Exoscale.
   - Creates a per-deployment SOS bucket for archive storage (when `var.s3_archive_enabled` is true) using the AWS S3 provider with a custom endpoint.
   - Creates an IAM role and API key granting scoped SOS access to the archive bucket; credentials are delivered via cloud-init.
   - Attaches the data block storage volume to each instance; cloud-init user data is injected.

2. Cloud-init (on each node):
   - Updates packages and installs minimal tools.
   - Downloads the `c4` installer binary and marks it executable.
   - Writes udev rules to expose the data volume as `/dev/exasol_data_01` and reloads rules.
   - Writes preparation and installation scripts to `/opt` and creates a readiness marker `/var/lib/exasol_launcher/state/cloud-init.complete`.
   - Writes SOS credentials (access key and secret key) from cloud-init metadata for archive volume registration.

3. Node initialization:
   - Cloud-init renders the assets from the installation preset into `/opt/exasol_launcher/`.
   - systemd units drive the unattended install via `exasol_launcher.target`.
   - Scripts and templates encapsulate preparation, installation, readiness checks, and remote archive registration (using the generated SOS credentials).

4. Outputs and local artifacts:
   - Public and private IPs, instance IDs per node.
   - SSH access info per node and the sensitive SSH private key.
   - Database access info per node (main port `8563`, Admin UI at `https://<public_ip>:8443`).
   - Local files written to `var.infrastructure_artifact_dir` (the deployment directory root when used via the launcher):
     - `deployment.json` — summary of nodes and access info
     - `secrets.json` — generated credentials (sensitive)
     - `node_access.pem` — SSH private key (mode `0600`)

## Credentials
- Exasol Database and Admin UI passwords are generated randomly if not provided; outputs are sensitive.
- Remote archive access uses an Exoscale IAM role and API key pair created per deployment; credentials are delivered via cloud-init as `.exoscale.archive.*` configuration.

## IAM Policy Files
To run this module, the operator identity (the Exoscale API key applying Terraform) needs permissions to manage compute instances, private networks, security groups, SSH keys, block storage, SOS buckets (via IAM), and IAM roles/API keys. This repository includes example policies:

- `iam-policy.minimal.json` — Least-privilege policy for Exasol deployments.
- `iam-policy.broad.json` — Broad access for dev/test environments.

Choose the policy that matches your environment's security posture. The minimal policy is recommended for production.

Note: Exoscale IAM policies are configured via the Exoscale Portal or API, not as JSON files uploaded to the platform. These files are examples for constructing IAM roles via the Portal.

## Configuration (Key Variables)
- `cluster_size` — number of nodes (default: `1`).
- `instance_type` — Exoscale instance type (default: `standard.extra-large`).
- `zone` — Exoscale zone (required; no default; set via `TF_VAR_zone`).
- `os_volume_size`, `data_volume_size` — root/data sizes in GB (defaults: `100`, `100`).
- `s3_archive_enabled` — create SOS bucket for archive storage (default: `true`).
- `power_state` — instance power state: `running` or `stopped` (default: `running`).

## Notes and Limitations
- Security groups expose required ports publicly; restrict via security group rules or configure network policies as needed.
- SOS buckets are managed via the AWS S3 API (S3-compatible). Object tagging is not supported.
- Instance state (`running`/`stopped`) is managed directly on the compute instance resource; no separate instance state resource.
- IAM roles are scoped to SOS-only access; instances receive explicit access key/secret via cloud-init.
