## Context

The STACKIT preset still writes all installation and infrastructure overlay files inline into cloud-init. The desired STACKIT bootstrap model is the same high-level shape used elsewhere: object storage carries the large host file overlays, while cloud-config fragments and machine-readable JSON metadata stay embedded in cloud-init.

STACKIT already provisions S3-compatible object storage buckets and credentials for the optional archive feature. That existing S3 compatibility makes bootstrap object storage a natural fit for the provider as well.

## Goals / Non-Goals

**Goals:**
- Add bootstrap object storage and remote bootstrap file delivery to the STACKIT preset.
- Keep `installation/cloudconf/*`, `/etc/exasol_launcher/infrastructure.json`, and `/etc/exasol_launcher/node.json` embedded in cloud-init.
- Upload `installation/files/**` and `assets/infrastructure/stackit/files/**` into a dedicated STACKIT bootstrap bucket and switch those host file overlays to `write_files.source.uri`.
- Manage STACKIT bootstrap objects through an aliased MinIO provider configured against the STACKIT Object Storage endpoint.
- Limit bootstrap object reads to the deployment servers by applying an object storage policy scoped to the servers' public IP addresses.
- Keep bootstrap storage lifecycle bound to the deployment, including cleanup on destroy.

**Non-Goals:**
- Changing the launcher-owned deployment directory contract.
- Refactoring the installation scripts or systemd workflow.
- Changing STACKIT archive bucket behavior.
- Introducing a second bootstrap transport model such as in-instance authenticated downloads.

## Decisions

### Use a dedicated STACKIT bootstrap bucket and credential set

The STACKIT preset will create a separate object storage bucket and credential group for bootstrap assets instead of reusing the archive bucket and archive credentials.

Rationale:
- Bootstrap distribution and archive storage have different lifecycle and access-control concerns.
- Bootstrap storage must exist even when archive integration is disabled.
- Separate credentials avoid coupling archive permissions to bootstrap synchronization.

Alternatives considered:
- Reuse the archive bucket and credentials.
  Rejected because it couples independent concerns and leaves bootstrap distribution unavailable when archive storage is disabled.

### Keep cloud-init on direct HTTPS `source.uri` fetches

The STACKIT preset will switch only the file overlay assets to HTTPS `write_files.source.uri` entries and leave the embedded cloud-config fragments and JSON metadata unchanged.

Rationale:
- This keeps the cloud-init contract aligned with the existing provider behavior.
- It reduces user-data size without introducing a second-stage bootstrap fetch script.
- It preserves the current host-side installation flow.

Alternatives considered:
- Use an authenticated bootstrap downloader on the server.
  Rejected because it would introduce a separate STACKIT-only bootstrap model and more runtime moving parts.

### Manage bootstrap objects through an aliased MinIO provider

Because STACKIT Object Storage is S3-compatible, the preset will manage bootstrap buckets and objects through an aliased `minio` provider configured against the STACKIT Object Storage endpoint.

Rationale:
- It keeps bootstrap object uploads first-class in Terraform state.
- It avoids custom apply-time helper scripts and local CLI dependencies.
- It preserves the current STACKIT UX where credentials are created per deployment by the preset.

Alternatives considered:
- Use a local sync helper fed by STACKIT-generated credentials.
  Rejected because it adds custom apply-time tooling and is less maintainable than first-class Terraform resources.
- Use the AWS provider for object uploads.
  Rejected because the provider behavior needed for this credential flow is less reliable for this preset design than the MinIO-based S3-compatible path.

### Restrict bootstrap reads with a bucket policy scoped to server public IPs

The STACKIT bootstrap bucket will remain private except for anonymous `GetObject` requests that originate from the current deployment servers' public IP addresses.

Rationale:
- STACKIT object storage does not provide AWS-style VPC endpoints or Azure-style subnet firewall plus signed URL combinations for this use case.
- Cloud-init still needs plain HTTPS fetches without request signing.
- The servers' public IPs are known during apply and can be expressed in the bucket policy.

Alternatives considered:
- Public bootstrap objects.
  Rejected because the goal is to keep bootstrap storage deployment-scoped rather than internet-readable.
- Presigned URLs.
  Rejected because time-bound URLs would add expiry and drift concerns to static cloud-init data.

## Risks / Trade-offs

- [STACKIT object storage access is limited by public IP rather than a private network path] -> Document the provider-specific access model and scope the bucket policy to the current deployment servers only.
- [MinIO provider support becomes part of the STACKIT preset surface] -> Keep the provider usage narrowly scoped to bootstrap bucket/object management and verify it with focused STACKIT validation.
- [Destroy can fail if bootstrap objects remain in the bucket] -> Use first-class Terraform object resources so destroy order removes objects before the bucket.
- [Public IP changes force policy and asset sync churn] -> Treat the server public IP set as part of the bootstrap sync trigger so access policy refreshes automatically.
