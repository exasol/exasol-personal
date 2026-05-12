# STACKIT Infrastructure as Code Architecture

## Overview
This document describes the Infrastructure as Code (IaC) implementation for Exasol Personal on STACKIT. It supports both single-node and multi-node (cluster) deployments with a simple, opinionated setup for networking, storage, and installation.

## Prerequisites and STACKIT Provider
- The service account key is taken from the environment variable `STACKIT_SERVICE_ACCOUNT_KEY_PATH`.
- The project ID is taken from the tofu variable `var.project_id`.
- The STACKIT provider uses this environment variable, while the S3-compatible bootstrap provider uses deployment-scoped object storage credentials created by the preset.

## Infrastructure Components

### Compute
- Servers named after their node IDs (e.g., `n11`, `n12`, ...).
- Default machine type `m2i.4` (memory-optimized; configurable via `var.instance_type`).
- Ubuntu image: selects the latest Ubuntu image for `var.ubuntu_version` unless `var.image_id` is provided.
- Cluster support: one instance per node; controlled by `var.cluster_size` (default: 1).

### Storage
- Separate volumes for OS and database data.
- Volume performance class for OS and database configurable via `var.volume_performance_class`.
- OS disk size configurable via `var.os_volume_size` (minimum 10 GB).
- Data volume size configurable via `var.data_volume_size` (in GB).
- A dedicated bootstrap object storage bucket is created for installation and infrastructure overlay files.
  - Bootstrap credentials are created per deployment and used through an S3-compatible provider to upload the overlay objects.
  - Bootstrap object reads are limited by bucket policy to the deployment servers' public IP addresses.
- A remote archive volume on STACKIT object storage (S3-compatible object storage) is created and registered automatically if `var.s3_archive_enabled` is true (default).
  - A per-deployment object storage bucket is created using a globally unique name.
  - Access to the bucket is granted via an API key pair delivered to servers via cloud-init.
- Data volume is attached as `/dev/sdf`. A udev rule creates `/dev/exasol_data_01`, which is referenced by the installer (c4).

### Networking
- **Simplified Model**: STACKIT instances receive public IP addresses automatically. No VPC, Internet Gateway, or route tables are needed.
- A managed private network is created for inter-node communication with static DHCP leases.
- Private IPs are assigned from the `172.30.1.0/24` range.
- Security groups control inbound/outbound traffic. STACKIT security groups are stateful and use separate rule resources.
- **No VPC Endpoints**: S3 access goes directly over HTTPS; no private network isolation for object storage.

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
- Labels (key/value pairs) are applied to resources that support them. Not all resources support labels (e.g., S3 bucket and S3 credentials group).
- Labels include:
  - `ManagedBy = "opentofu"`
  - `Project = "exasol-personal"`
  - `Deployment = <deployment_id>`
  - `CreatedAt  = <timestamp>`

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
   - Stores the private key as a sensitive output, writes it to a local PEM file.
   - Provisions the private network, security group/rules, compute instances, and block storage volumes.
   - Starts servers, with the SSH public key attached.
   - Creates a per-deployment bootstrap object storage bucket and uploads the installation and infrastructure overlay files needed at first boot.
   - Creates a per-deployment object storage bucket for archive storage (when `var.s3_archive_enabled` is true).
   - Attaches the data block storage volume to each server; cloud-init user data is injected.

2. Cloud-init (on each node):
   - Updates packages and installs minimal tools.
   - Downloads the `c4` installer binary and marks it executable.
   - Writes udev rules to expose the data volume as `/dev/exasol_data_01` and reloads rules.
   - Fetches the installation and infrastructure overlay files from the bootstrap bucket and creates a readiness marker `/var/lib/exasol_launcher/state/cloud-init.complete`.

3. Node initialization:
   - Cloud-init keeps the installation cloud-config fragments and JSON metadata embedded, and fetches the larger host file overlays over HTTPS from object storage.
   - systemd units drive the unattended install via `exasol_launcher.target`.
   - Scripts and templates encapsulate preparation, installation, readiness checks, and remote archive registration (using the generated S3 credentials).

4. Outputs and local artifacts:
   - Public and private IPs, instance IDs, and DNS names per node.
   - SSH access info per node and the sensitive SSH private key.
   - Database access info per node (main port `8563`, Admin UI at `https://<public_ip>:8443`).
      - Local files written to `var.infrastructure_artifact_dir` (the deployment directory root when used via the launcher):
      - `deployment.json` — summary of nodes and access info
      - `secrets.json` — generated credentials (sensitive)
      - `node_access.pem` — SSH private key (mode `0600`)

## Credentials
- Exasol Database and Admin UI passwords are generated and injected; outputs are sensitive.
- Bootstrap object storage uses deployment-scoped credentials created by the preset; servers consume the objects through HTTPS URLs constrained by bucket policy to the servers' public IPs.
- Remote archive access uses the object storage credentials delivered via cloud-init as `stackit.archive.*` configuration; no user-supplied keys are required.

## Configuration (Key Variables)
- `cluster_size` — Number of nodes (default: `1`).
- `instance_type` — STACKIT machine type (memory-optimized; default: `m2i.4`).
- `ubuntu_version` — Ubuntu version (default: `22.04`).
- `image_id` — Optional explicit image override (skips auto-selection).
- `volume_performance_class` — Volume performance class for OS and data (default: `storage_premium_perf6`).
- `os_volume_size`, `data_volume_size` — Root and data sizes in GB (defaults: `100`, `100`).

## Notes and Limitations
- Security groups expose required ports publicly; restrict via security group rules or configure network policies as needed (e.g. `var.allowed_cidr`).
- Server state (`running`/`stopped`) is managed directly on the server resource; no separate server state resource.
- STACKIT bootstrap storage access is scoped by public IP because the preset has no provider feature equivalent to AWS VPC endpoints or Azure subnet-restricted blob access for cloud-init bootstrap reads.
- STACKIT does not support creating IAM roles or policies for this flow, so bootstrap and archive access rely on object storage credentials and bucket policy instead.
