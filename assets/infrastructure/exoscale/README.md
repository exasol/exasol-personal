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
- Ubuntu template: uses `Linux Ubuntu 24.04 LTS 64-bit` from Exoscale's public template catalog.
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
- **Simplified Model**: Exoscale instances receive public IP addresses automatically. No Internet Gateway or route tables are needed.
- A VPC with one subnet (`172.16.0.0/22`) is created for inter-node communication, and each instance is attached to it with a fixed private IP.
- A VPC is a routed (layer 3) network that carries only IPv4 unicast traffic. Exasol's cluster communication works over unicast, but the VPC needs the interface configuration described in [VPC Interface Configuration](#vpc-interface-configuration).
- Security groups control inbound/outbound traffic, including traffic inside the VPC. Exoscale security groups are stateful and use separate rule resources.
- **No VPC Endpoints**: SOS access goes directly over HTTPS to `sos-<zone>.exo.io`; no private network isolation for object storage.
- Deployments created before VPC support use an Exoscale Private Network. Their deployment directories keep the preset version they were created with, so they can still be stopped, started, and destroyed.

### VPC Interface Configuration
Cloud-init writes a netplan file for the secondary interface (`eth1`) of each node instead of using the VPC's DHCP configuration. Multi-node clusters need it for two reasons.

**Subnet prefix and routes.** VPC DHCP assigns each node a `/32` address and a route to the subnet through the gateway (`172.16.0.1`). The Exasol cluster daemon (cored) discards packets from peers outside its interface's subnet, so with a `/32` address it logs `Discard packet from different subnet` for every other node, and the cluster never forms. Assigning the address with the subnet prefix (`/22`) alone does not help: the kernel then treats the other nodes as on-link and resolves them by ARP, but the VPC answers ARP only for its gateway. The netplan file therefore assigns `<node-ip>/22` and adds two `on-link` routes for the halves of the subnet (`172.16.0.0/23` and `172.16.2.0/23`) through the gateway. Being more specific, they take precedence over the `/22` prefix route, so all traffic to other nodes goes through the gateway while cored sees them in its subnet.

**Jumbo MTU.** The VPC and security group datapath drops IP fragments with fewer than 8 payload bytes. Measured with UDP datagrams between two nodes at the default MTU of 1500, datagrams of 1473 to 1479 and 2953 to 2959 bytes never arrive, while all other sizes do. The same happens on the public interface, and a security group rule that allows all protocols does not change it. The cluster's membership protocol (Totem) sends UDP messages of several kilobytes and retransmits a lost message indefinitely, so a two-node cluster stalls: cored logs `Failed to receive message <n> for multiple times` and the database never boots. With an MTU large enough for these messages, they are not fragmented at all.

The VPC carries IPv4 packets of up to 8930 bytes, which is the default of the private variable `vpc_mtu`. Exoscale does not document this value; it is the largest IP packet size that passed a ping with the don't-fragment flag set between two nodes of a VPC. If Exoscale changes it, packets above the new limit are silently lost and multi-node clusters stall again with the symptom above. To re-measure it, run on one node, with the other node's VPC address:

```bash
lo=1472; hi=9000
while [ $((hi - lo)) -gt 1 ]; do
  m=$(((lo + hi) / 2))
  if ping -c1 -W2 -M do -s "$m" 172.16.0.21 >/dev/null 2>&1; then lo=$m; else hi=$m; fi
done
echo "MTU $((lo + 28))"
```

The interface MTU must be at least that large for the probe to send the packets, for example `sudo ip link set eth1 mtu 9000` before the measurement. To deploy with a different value, set `vpc_mtu` in `variables_private.tf` of a copy of this preset.

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
  - VPCs and VPC subnets
  - Block storage volumes
- Labels include:
  - `name = <resource_name>`
  - `deployment_id = <deployment_id>`
- Not all resources support labels (e.g., security groups, SSH keys, IAM roles).

## Node Addressing Scheme
- Nodes are identified as `n<NN>` starting at `n11`.
- Private IPs are statically assigned in the VPC subnet: `172.16.0.<20 + i>` where `i` ranges from `0` to `var.cluster_size - 1`.
- Example for `cluster_size = 3`:
  - `n11` → `172.16.0.20`
  - `n12` → `172.16.0.21`
  - `n13` → `172.16.0.22`
- These addresses are passed to the installer for cluster configuration.

## Deployment Lifecycle
1. Terraform plan/apply:
   - Generates a deployment ID and a new ECDSA SSH key pair.
   - Provisions the VPC and its subnet, security group/rules, compute instances, and block storage volumes.
   - Registers the SSH public key with Exoscale.
   - Creates a per-deployment SOS bucket for archive storage (when `var.s3_archive_enabled` is true) using the AWS S3 provider with a custom endpoint.
   - Creates a separate per-deployment SOS bucket named with the `boostrap` suffix and uploads the installation and infrastructure file overlays used by cloud-init.
   - Creates an IAM role and API key granting scoped SOS access to the archive bucket; credentials are delivered via cloud-init.
   - Attaches the data block storage volume to each instance; cloud-init user data is injected.

2. Cloud-init (on each node):
   - Updates packages and installs minimal tools.
   - Downloads the `c4` installer binary and marks it executable.
   - Writes udev rules to expose the data volume as `/dev/exasol_data_01` and reloads rules.
   - Fetches preparation and installation assets from direct HTTPS SOS object URLs, writes them to the target paths on the host, and creates a readiness marker `/var/lib/exasol_launcher/state/cloud-init.complete`.
   - Writes SOS credentials (access key and secret key) from cloud-init metadata for archive volume registration.

3. Node initialization:
   - Cloud-init renders embedded metadata and downloads bootstrap assets into `/opt/exasol_launcher/` and other target paths.
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

## IAM Role and API Key
The operator identity (the Exoscale API key applying OpenTofu) needs a role that allows these IAM service classes:

- `compute` for instances, block storage, security groups, SSH keys, and the Private Networks of deployments created before VPC support.
- `networking` for the VPC and subnet that connect the nodes.
- `sos` for the bootstrap and archive buckets.
- `iam` for the role and API key that grant the nodes access to the archive bucket.

A key without `networking` fails with `403 Forbidden by role policy for networking`. The built-in Owner role allows every service by default, while a role with the `deny` default strategy must list `networking` explicitly. The Exoscale Portal's role editor does not offer `networking`, so create or update such a role through the API (`create-iam-role`, `update-iam-role-policy`) or the [`exo` CLI](https://community.exoscale.com/reference/cli/exo/iam/role/).

This directory includes two policies in the API's `iam-policy` format, which the API and the CLI accept as they are:

- `iam-policy.minimal.json` - least-privilege policy for Exasol deployments, recommended for production.
- `iam-policy.broad.json` - allows each required service in full, for dev/test environments.

For example, create a role from the minimal policy and an API key bound to it with the CLI:

```bash
exo iam role create exasol-launcher --description "Exasol Personal launcher" --policy - < iam-policy.minimal.json
exo iam api-key create exasol-launcher exasol-launcher
```

To add `networking` to an existing role, replace its policy with `exo iam role update <role> --policy - < iam-policy.minimal.json`. Store the key and secret printed by `exo iam api-key create`, as the secret is shown only once.

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
- Bootstrap asset delivery uses a separate HTTPS-readable SOS bucket so cloud-init payload size stays small.
- Exoscale SOS does not currently provide an instance-scoped private-access mechanism analogous to the AWS or Azure presets, so bootstrap assets continue to use direct HTTPS object URLs.
