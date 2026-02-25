# AWS Infrastructure as Code Architecture

## Overview
This document describes the Infrastructure as Code (IaC) implementation for Exasol Personal on AWS. It supports both single-node and multi-node (cluster) deployments with a simple, opinionated setup for networking, storage, and installation.

## Prerequisites and AWS Provider
- AWS credentials and region are taken from environment variables. At minimum, set one of `AWS_REGION` or `AWS_DEFAULT_REGION`, and provide credentials via `AWS_PROFILE` or `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` (and `AWS_SESSION_TOKEN` if applicable).
- The provider configuration in `providers.tf` doesn't define AWS region, AZs or credentials by design; it relies on the AWS SDK environment/config chain.

## Infrastructure Components

### Compute
- EC2 instances named after their node IDs (e.g., `n11`, `n12`, ...).
- Default instance type `r6i.xlarge` (memory-optimized; configurable via `var.instance_type`).
- Ubuntu AMI: selects the latest Ubuntu image for `var.ubuntu_version` unless `var.ami_id` is provided.
- Cluster support: one instance per node; controlled by `var.cluster_size` (default: 1).

### Storage
- Separate EBS volumes for OS/root and database data.
- Volume type defaults to `gp3`; sizes configurable via `var.os_volume_size` and `var.data_volume_size`.
- Data volume is encrypted and attached as `/dev/sdf`. A udev rule creates `/dev/exasol_data_01`, which is referenced by the installer (c4).
- A remote archive volume on Amazon S3 is created and registered automatically if `var.s3_archive_enabled` is true (default).
   - A per-deployment S3 bucket is created using a globally unique name.
   - Access to the bucket is granted to the EC2 instances via an instance role and instance profile (no long-lived access keys).

### Networking
- Single VPC with one public subnet (IPv4 only), an Internet Gateway, and a route to `0.0.0.0/0`.
- All intra-VPC traffic is allowed.
- Public connectivity is via instance public IPs/DNS from the public subnet (no Elastic IPs are used by default).
- **Availability Zone Selection**: The subnet is automatically placed in an availability zone that supports the chosen instance type. This prevents deployment failures when an instance type is unavailable in a randomly selected AZ. The selected AZ is shown in outputs.
- An S3 Gateway VPC Endpoint is created automatically when a remote archive volume on Amazon S3 is created, keeping S3 traffic on the AWS backbone and supporting private subnets.

## Access and Security
The following ports must be reachable from the operator’s network, controlled via `var.allowed_cidr` (defaults to `0.0.0.0/0`; restrict in real use):

1. 22 — SSH access
2. 2581 — Default bucketfs
3. 8443 — Admin UI (HTTPS)
4. 8563 — Default database port
5. 20002 — Exasol container SSH
6. 20003 — Exasol confd API

## Resource Organization (Tagging)
- A unique deployment ID is generated at apply time (e.g., `exasol-<hex>`).
- Tags on all resources include (via provider default_tags):
  - `ExasolTool = "exasol"`
  - `Deployment = <deployment_id>`
  - `Owner = <AWS caller identity ARN>`
- Resource names include the deployment ID to avoid cross-linking across deployments.

## Node Addressing Scheme
- Nodes are identified as `n<NN>` starting at `n11`.
- Private IPs are fixed and derived from the subnet: `172.30.1.<11 + i>` where `i` ranges from `0` to `var.cluster_size - 1`.
- Example for `cluster_size = 3`:
  - `n11` → `172.30.1.11`
  - `n12` → `172.30.1.12`
  - `n13` → `172.30.1.13`
- These addresses are passed to the installer for cluster configuration.

## Deployment Lifecycle
1. Terraform plan/apply:
   - Generates a deployment ID and a new RSA SSH key pair.
   - Stores the private key as a sensitive output, writes it to a local PEM file, and stores it in SSM Parameter Store at `/${deployment_id}/ssh_private_key`.
   - Provisions the VPC, subnet, route table, IGW, security group, data volumes, EC2 instances, and the S3 VPC endpoint (automatically when `var.s3_archive_enabled` is true).
   - Creates a per-deployment S3 bucket for archive storage (when `var.s3_archive_enabled` is true).
   - Creates an EC2 instance role and instance profile granting scoped S3 access to the archive bucket; no IAM users or access keys are created.
   - Attaches the data EBS volume to each node; cloud-init user data is injected.

2. Cloud-init (on each node):
   - Updates packages and installs minimal tools.
   - Downloads the `c4` installer binary and marks it executable.
   - Writes udev rules to expose the data volume as `/dev/exasol_data_01` and reloads rules.
   - Writes preparation and installation scripts to `/opt` and creates a readiness marker `/var/lib/exasol_launcher/state/cloud-init.complete`.

3. Node initialization:
   - Cloud-init renders the assets from the installation preset into `/opt/exasol_launcher/`.
   - systemd units drive the unattended install via `exasol_launcher.target`.
   - Scripts and templates encapsulate preparation, installation, readiness checks, and remote archive registration (using the generated S3 credentials).

4. Outputs and local artifacts:
   - Public and private IPs, instance IDs, and DNS names per node.
   - SSH access info per node and the sensitive SSH private key.
   - Database access info per node (main port `8563`, Admin UI at `https://<dns>:8443`).
    - Local files written to `var.infrastructure_artifact_dir` (the deployment directory root when used via the launcher):
     - `deployment-<deployment_id>.json` — summary of nodes and access info
       - `secrets-<deployment_id>.json` — generated credentials (sensitive)
     - `<deployment_id>.pem` — SSH private key (mode `0600`)

## Credentials
- Exasol Database and Admin UI passwords are generated and injected; outputs are sensitive.
- Remote archive access uses the EC2 instance role attached to the nodes; no user-supplied keys are required, and no long-lived S3 credentials are created.

## IAM Policy Files
To run this module, the operator identity (the AWS principal applying Terraform) needs permissions to manage EC2/VPC, S3 (archive bucket), IAM roles/instance profiles, SSM parameters, and S3 VPC endpoints. This repository includes example policies:

- `assets/infrastructure/aws/iam-policy.minimal.json` — Least-privilege policy scoped to Exasol resources (roles/instance profiles named `exasol-*`, SSM params under `/${deployment_id}/…`, S3 buckets named `exasol-*`).
- `assets/infrastructure/aws/iam-policy.broad.json` — Broad but scoped: full access to key services (EC2, S3, IAM, SSM) constrained to Exasol resource naming patterns where possible.

Choose the policy that matches your environment’s security posture. The minimal policy is recommended for production.

## Configuration (Key Variables)
- `cluster_size` — number of nodes (default: `1`).
- `instance_type` — EC2 instance type (default: `r6i.xlarge`).
- `ubuntu_version` — Ubuntu codename; selects latest AMI (default: `jammy`).
- `ami_id` — optional explicit AMI override (skips auto-selection).
- `volume_type` — EBS type for root/data (default: `gp3`).
- `os_volume_size`, `data_volume_size` — root/data sizes in GB (defaults: `100`, `100`).

## Notes and Limitations
- Security groups expose only the required ports; restrict `allowed_cidr` for secure use.
- Public connectivity currently uses instance public IPs/DNS from the public subnet. Elastic IPs can be introduced if static addressing is desired.
- No EC2 instance role is required for S3 access; Exasol uses generated IAM user credentials.
- The operator identity running Terraform must be permitted to manage: S3 buckets, IAM users and access keys (for the per-deployment S3 writer), and VPC endpoints.
