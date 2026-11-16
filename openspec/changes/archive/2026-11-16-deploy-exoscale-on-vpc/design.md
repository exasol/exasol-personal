## Context

Exoscale stopped accepting `create-private-network` for new workloads in late October 2026 and recommends VPC. A VPC is a routed (layer 3) network: it carries only IPv4 unicast traffic, ARP is answered by the platform, and instances see their VPC address as a `/32` with routes delivered by DHCP. Security groups filter VPC traffic, unlike Private Network traffic. VPC management is a separate IAM service, `networking`.

The Exoscale Terraform provider supports VPCs (`exoscale_vpc` since 0.72, `exoscale_vpc_subnet` since 0.73) and attaching instances to subnets with fixed addresses (`exoscale_compute_instance.vpc`) since 0.76.

## Goals / Non-Goals

**Goals:** new Exoscale deployments succeed; existing Private Network deployments stay manageable; the nightly cleanup removes all Exoscale leftovers older than its threshold.

**Non-Goals:** migrating existing deployments to VPC.

## Decisions

- **One VPC and one subnet per deployment.** A dedicated VPC isolates deployments from each other and from the organization's default VPC, and is destroyed with the deployment. The subnet keeps the previous `172.16.0.0/22` addressing and the node addresses `172.16.0.20` onward, which lie outside the reserved first addresses of a VPC subnet. The node addresses stay fixed, so the installer, the node barrier, and the Exasol cluster configuration are unchanged.
- **Unicast-only networking is sufficient.** The installer and the node barrier use SSH to fixed private addresses, and the Exasol cluster membership protocol exchanges unicast UDP with the configured peer addresses. Multi-node clusters already run on AWS VPCs and Azure virtual networks, which carry no broadcast or multicast either.
- **Security group rules allow intra-cluster traffic.** The existing self-referencing TCP, UDP, and ICMP echo rules, previously irrelevant for Private Network traffic, now admit the VPC traffic between nodes.
- **The secondary interface is configured statically.** VPC DHCP assigns a `/32` address, and the Exasol cluster discards packets from peers outside the interface's subnet, which left a two-node cluster without quorum. Cloud-init therefore writes a per-node netplan file that assigns the fixed address with the subnet prefix. The VPC answers ARP only for its gateway, so the two halves of the subnet are routed through the gateway, which takes precedence over the on-link route of the prefix. No default route is set on this interface.
- **The VPC interface uses jumbo frames.** The VPC drops IP fragments with fewer than 8 payload bytes, also with a security group rule for all protocols, so a UDP datagram whose last fragment is that small never arrives. The cluster membership protocol sends messages of several kilobytes and retransmits a lost one indefinitely, which stalled a two-node cluster at an MTU of 1500. The VPC carries IPv4 packets of up to 8930 bytes, so the interface uses that MTU and these messages are not fragmented.
- **Existing deployments keep their preset.** The launcher extracts the preset into the deployment directory at initialization, together with its provider lock file, so existing deployments keep using provider 0.68 and their Private Network; Exoscale still allows managing and deleting existing Private Networks.
- **Cleanup discovers from names.** Security groups, SSH keys, IAM roles and API keys, and SOS buckets carry the deployment id in their names but no owner. They establish a deployment only when the owner filter accepts any owner, as the nightly cleanup's `--owner=*` does. SOS lists the buckets of every zone, so each deployment bucket's zone is looked up. Security groups, SSH keys, and IAM resources belong to no zone; they establish a deployment only if it has no instances, volumes, private networks, VPCs, or buckets in another zone. VPCs and subnets carry `deployment_id` labels like instances and volumes. Subnets are deleted after instances, and VPCs after subnets.

## Risks / Trade-offs

- API keys whose role lacks `networking` fail to create the VPC. The changelog lists this as a breaking change and the troubleshooting guide names the error.
- Security groups, SSH keys, and IAM roles have no creation time, and they may belong to a deployment in the first seconds of its creation. An age-filtered cleanup therefore skips a deployment that only they reveal, so such leftovers stay until someone removes them by id; they incur no cost.
- The 8930-byte VPC MTU was measured, not documented by Exoscale. If Exoscale lowers it, packets above the new limit are silently lost and new multi-node deployments fail: the installation times out waiting for the database, and cored logs `Failed to receive message <n> for multiple times`. Single-node deployments and the single-node live smoke test are unaffected, so a regression shows only in multi-node deployments; the preset README describes how to re-measure the limit and override `vpc_mtu`.
- The dropping of small IP fragments is undocumented platform behavior. If Exoscale fixes it, the jumbo MTU is no longer required but remains harmless.
- The live smoke test deploys a single node, so cluster-internal VPC traffic is only exercised by multi-node deployments run manually.
