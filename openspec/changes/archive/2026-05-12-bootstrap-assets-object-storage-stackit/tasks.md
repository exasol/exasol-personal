## 1. STACKIT bootstrap storage resources

- [x] 1.1 Add a dedicated STACKIT bootstrap object storage bucket and bootstrap-specific credentials that exist independently of archive storage.
- [x] 1.2 Configure an aliased MinIO provider against the STACKIT Object Storage endpoint using STACKIT-generated bootstrap credentials.
- [x] 1.3 Manage bootstrap objects and bucket access policy through Terraform resources, including cleanup on destroy and public-IP-scoped read access.

## 2. Cloud-init bootstrap asset transport

- [x] 2.1 Build a shared STACKIT file-to-object-key mapping for installation and infrastructure overlay files using `dest_path` with the leading `/` stripped.
- [x] 2.2 Switch STACKIT overlay file delivery in `cloudinit.tf` from inline `write_files.content` entries to HTTPS `write_files.source.uri` entries.
- [x] 2.3 Preserve embedded delivery for `installation/cloudconf/*`, `/etc/exasol_launcher/infrastructure.json`, and `/etc/exasol_launcher/node.json`.

## 3. Documentation and specification alignment

- [x] 3.1 Update the STACKIT preset README to describe the bootstrap bucket lifecycle, object upload flow, and the public-IP-scoped access model.
- [x] 3.2 Update the `bootstrap-asset-distribution` specification so STACKIT is covered alongside the existing providers.
