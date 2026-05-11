## Context

All current infrastructure presets materialize host-side bootstrap assets by reading `installation/files/**` and provider-specific `files/**` from the extracted deployment directory and inlining them into cloud-init `write_files.content`. This keeps the bootstrap contract simple, but it pushes script and unit payload size into cloud-init user data and duplicates the same transport pattern across providers.

This change is cross-cutting because all three provider presets already have distinct object storage primitives and cloud-init templates, but the launcher-facing preset contract stays the same. The deployment directory still contains extracted installation and infrastructure assets; only the way those assets reach the instance changes.

## Goals / Non-Goals

**Goals:**
- Move host file asset transport from inline cloud-init content to provider object storage for AWS, Azure, and Exoscale.
- Use `write_files.source.uri` over HTTPS for all uploaded host file assets.
- Preserve the existing bootstrap phase model by leaving embedded cloud-config parts and embedded machine-readable JSON payloads unchanged.
- Keep lifecycle ownership inside the infrastructure presets so creation, update, and destroy remain part of normal OpenTofu operations.
- Use provider-native private-access controls for bootstrap storage where the provider supports instance- or subnet-scoped access.

**Non-Goals:**
- Changing the launcher-owned deployment directory contract.
- Moving `installation/cloudconf/*` into object storage.
- Changing the format or contents of `/etc/exasol_launcher/infrastructure.json` or `/etc/exasol_launcher/node.json`.
- Refactoring installation scripts or systemd workflow beyond what is needed to consume the same files from a different transport.

## Decisions

### Use a dedicated bootstrap object store per deployment

Each provider will create a separate object store for bootstrap assets rather than reusing the existing remote archive storage resources.

Rationale:
- Archive storage and bootstrap asset transport have different ownership, access, and lifecycle concerns.
- Azure archive storage is intentionally private and subnet-scoped for database archive use; reusing it would blur concerns and complicate access policy changes.
- A dedicated store makes destroy behavior and documentation clearer.

Alternatives considered:
- Reuse existing archive storage resources.
  Rejected because it couples independent concerns and would force archive-specific access rules onto bootstrap transport.

### Upload only `installation/files/**` and provider `files/**`

The change will only externalize file overlay assets that currently map onto host filesystem paths. The installation preset `cloudconf/*` fragments remain embedded as multipart cloud-init parts, and the generated JSON metadata files remain embedded as inline content.

Rationale:
- This gives the size reduction where the bulk payload exists today without changing the cloud-init phase model.
- Embedded JSON metadata stays immediately available without any remote fetch dependency.
- Embedded cloud-config parts avoid introducing second-stage cloud-init composition or remote include behavior.

Alternatives considered:
- Move all cloud-init payloads into object storage.
  Rejected because it would change bootstrap semantics and introduce unnecessary complexity.

### Use the host path with its leading slash stripped as the object key

Uploaded objects use the host `dest_path` with its leading `/` stripped, for example `/opt/exasol_launcher/scripts/installExasol.sh` becomes `opt/exasol_launcher/scripts/installExasol.sh`.

Rationale:
- The installation and infrastructure file sets are guaranteed not to collide.
- The object key stays directly derived from the host path while remaining valid for object-store URL paths.
- Object keys remain easy to inspect during debugging because they mirror host paths.

Alternatives considered:
- Use the absolute `dest_path` as the object key.
  Rejected because leading `/` characters produce fragile object URLs for S3-style fetches.

### Use HTTPS object URLs that match each provider implementation

Cloud-init fetches bootstrap assets over HTTPS `write_files.source.uri` values. AWS uses the bucket regional domain name with bucket access restricted to the deployment S3 VPC endpoint, Azure uses blob URLs with signed access into a private container, and Exoscale keeps the explicit SOS endpoint URL shape.

Rationale:
- This keeps the cloud-init contract uniform while still letting each provider use its most practical URL form.
- Azure already exposes a full blob URL, AWS exposes a bucket regional domain name, and Exoscale relies on a custom S3-compatible endpoint.
- AWS and Azure have provider-native controls that keep bootstrap object reads scoped to the deployment network path without changing the cloud-init contract.

Alternatives considered:
- Uniform private transport across all providers.
  Rejected because Exoscale SOS does not provide an instance-scoped private-access mechanism analogous to S3 VPC endpoint policies or Azure Storage firewall and SAS combinations.

### Use direct HTTPS SOS object URLs on Exoscale

The Exoscale preset uses the direct HTTPS SOS object URL flow and does not attempt to mirror the AWS or Azure private-access controls.

Rationale:
- Exoscale SOS uses S3-compatible object URLs but does not expose an instance-scoped network restriction equivalent to the AWS or Azure mechanisms used here.
- Forcing a different transport just for Exoscale would add a second bootstrap model and increase operational complexity.

Alternatives considered:
- Presigned SOS URLs or per-instance authenticated bootstrap downloads.
  Rejected for now because they add extra signing or credential-distribution logic and were not accepted for this implementation.

### Keep lifecycle fully inside provider presets

Object store creation, object upload/update, signed URL generation, and object store destruction remain part of the infrastructure presets.

Rationale:
- The launcher should continue to treat presets as self-contained infrastructure implementations.
- Terraform already owns the extracted local file paths and remote provider resources needed to express drift and cleanup.

Alternatives considered:
- Pre-upload files from the launcher before `tofu apply`.
  Rejected because it would create a new launcher-to-provider responsibility boundary and duplicate provider logic outside the presets.

## Risks / Trade-offs

- [Exoscale cannot match the AWS/Azure private-access posture in the current design] -> Document the limitation explicitly and keep the Exoscale transport isolated from the AWS and Azure private-access assumptions.
- [Provider URL generation differs across AWS, Azure, and Exoscale] -> Keep the high-level cloud-init contract identical and use provider-native conveniences where they exist.
- [Object updates may not propagate if Terraform resource dependencies are incomplete] -> Derive upload resources directly from file enumerations and content hashes so asset changes force object and URL refresh.
- [Destroy can fail if object stores are not emptied first] -> Use provider resource settings or explicit object resources so Terraform can remove objects before deleting the bucket or container.
- [HTTPS bootstrap fetches can fail if object keys do not map cleanly to URLs] -> Derive object keys from `dest_path` with the leading `/` stripped so URL paths remain stable.
