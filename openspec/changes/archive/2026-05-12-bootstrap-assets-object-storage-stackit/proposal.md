## Why

The STACKIT infrastructure preset still embeds all installation and infrastructure overlay files directly into cloud-init user data. That keeps large script payloads coupled to cloud-init size limits and prevents STACKIT from shipping bootstrap assets through object storage the way the provider can already do for archive data.

## What Changes

- Add deployment-scoped bootstrap object storage to the STACKIT infrastructure preset for `installation/files/**` and `assets/infrastructure/stackit/files/**`.
- Switch STACKIT cloud-init host file overlays from inline `write_files.content` entries to HTTPS `write_files.source.uri` entries.
- Keep `installation/cloudconf/*`, `/etc/exasol_launcher/infrastructure.json`, and `/etc/exasol_launcher/node.json` embedded in cloud-init.
- Use a MinIO provider configured against STACKIT Object Storage so bootstrap objects can be managed with S3-compatible Terraform resources while STACKIT-generated credentials remain deployment-scoped.
- Restrict STACKIT bootstrap object reads to the deployed servers by applying an object storage policy scoped to the servers' public IP addresses.
- Document the STACKIT-specific bootstrap transport and access-control behavior alongside the existing provider variants.

## Capabilities

### New Capabilities

### Modified Capabilities
- `bootstrap-asset-distribution`: Add STACKIT-specific bootstrap object storage, transport, and access-control requirements.

## Impact

- Affected infrastructure code in `assets/infrastructure/stackit`.
- Cloud-init generation changes in `assets/infrastructure/stackit/cloudinit.tf`.
- New bootstrap object storage resources, MinIO-provider object management, and provider documentation for STACKIT.
- Updates to the bootstrap asset distribution specification and STACKIT implementation plan.
