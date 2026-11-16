## 1. Exoscale VPC networking

- [x] 1.1 Replace the Private Network of the Exoscale preset with a per-deployment VPC and subnet, attach nodes with fixed addresses, configure the VPC interface with the subnet prefix and an 8930-byte MTU, upgrade the provider to 0.77, add the `networking` service to the IAM example policies, document the IAM role and API key setup, and update the user documentation, troubleshooting guide, and changelog. Commit: `fix: Deploy Exoscale clusters on VPC - SPOT-34037`.

## 2. Orphaned deployment cleanup

- [x] 2.1 Discover Exoscale deployments from VPCs, subnets, SOS buckets, and organization-wide resources, resolve bucket zones, and delete subnets and VPCs after instances, with unit tests. Commit: `feat(cleanup): Find Exoscale deployments from any leftover resource - SPOT-34037`.

## 3. Validation

- [x] 3.1 `task fmt`, `task lint`, `task tests-unit`, `task tests-launcher`, the cleanup tool's unit tests, `task docs-build`, and `openspec validate deploy-exoscale-on-vpc --strict` pass.
- [x] 3.2 A dry run of `exasol-cleanup run --all --older-than=24h --provider=exoscale --owner=*` plans the deletion of exactly the five leftover deployments' buckets.
- [x] 3.3 A single-node and a two-node `exasol install exoscale` succeed, the two-node database runs on both nodes (`SELECT NPROC()` returns 2), `exasol stop` and `exasol start` work, `exasol destroy` leaves no resources, and the Exoscale deployment test workflow passes.
