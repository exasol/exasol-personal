# Hetzner Cloud Infrastructure as Code Architecture

## Overview
This document describes the Infrastructure as Code (IaC) implementation for Exasol Personal on Hetzner Cloud. It supports both single-node and multi-node (cluster) deployments with a simple, opinionated setup for networking, storage, and installation.

## Prerequisites and Hetzner Cloud Provider
- Hetzner Cloud API token is taken from the environment variable `HCLOUD_TOKEN`.
- Location selection is available via `--location` (e.g., `fsn1`, `nbg1`, `hel1`, `ash`, `hil`). Defaults to `fsn1` (Falkenstein).
- The provider configuration in `providers.tf` uses the environment variable and does not define credentials inline.

## Infrastructure Components

### Compute
- Hetzner Cloud servers named after their node IDs (e.g., `n11`, `n12`, ...).
- Default server type `ccx33` (8 vCPU / 16 GB RAM; configurable via `--server-type`).
- Ubuntu 22.04 image from Hetzner's public image catalog.
- Cluster support: one server per node; controlled by `--cluster-size` (default: 1).
- Servers get public IPv4 addresses automatically.

### Storage
- Separate volumes for OS/root and database data.
- OS disk is part of the server (size configurable via `--os-volume-size`, minimum 20 GB).
- Data volumes are separate block storage volumes attached via `hcloud_volume_attachment`.
- Data volume size configurable via `--data-volume-size` (in GB).
- Remote archive volume on Hetzner Object Storage (S3-compatible) can be enabled via `--s3-archive-enabled` (default: disabled). Available in EU locations only.

### Networking
- Servers receive public IPv4 addresses automatically.
- A private network (`hcloud_network`) with a subnet is created for inter-node communication.
- Private IPs are assigned from the `172.16.0.0/24` range starting at `.20`.
- A firewall (`hcloud_firewall`) controls inbound/outbound traffic.

## Access and Security
The following ports are exposed via firewall rules:

1. 22 — SSH access (public)
2. 2581 — Default bucketfs (public)
3. 8443 — Admin UI HTTPS (public)
4. 8563 — Default database port (public)
5. 20002 — Exasol container SSH (public)
6. 20003 — Exasol confd API (public)

Additionally, internal traffic (TCP/UDP/ICMP) is allowed between cluster nodes via the private network CIDR range.

## Resource Organization (Labels)
- A unique deployment ID is generated at apply time (e.g., `exasol-<deployment_id>`).
- All resources carry labels: `managed_by=opentofu`, `project=exasol-personal`, `deployment_id=exasol-<id>`.
- The cleanup tool uses these labels to discover and manage deployments.

## Deployment Artifacts
After a successful apply, the following files are written to the deployment directory:
- `deployment.json` — connection info (IPs, ports, instance IDs)
- `secrets.json` — credentials (DB password, Admin UI password)
- `node_access.pem` — SSH private key for node access
