## Context

Provider presets upload launcher files to deployment-scoped object storage and reference them from cloud-init `write_files` entries. The current entries are processed before the launcher starts, but a transient early-boot network failure can leave a required file absent while cloud-init continues with a warning.

## Goals / Non-Goals

**Goals:**

- Move remote bootstrap asset retrieval to cloud-init's final stage.
- Keep bootstrap files in provider object storage rather than increasing user-data with inline file contents.
- Validate the complete provider-specific asset set before launcher startup.
- Preserve the existing launcher workflow and provider access controls.

**Non-Goals:**

- Adding a second asset transport or changing object-store permissions.
- Retrying installation automatically after a bootstrap failure.
- Changing launcher scripts, service dependencies, or asset contents.

## Decisions

### Defer remote file writes

Mark every object-storage-backed `write_files` entry with `defer: true`. Cloud-init's native deferred-write module runs after package setup and before user scripts in the final stage, reducing exposure to early network initialization without adding payload content.

An inline fallback is rejected because the launcher asset set is large and the existing design intentionally avoids the provider user-data limit.

### Start the launcher from a final-stage shell part

Remove the shared `runcmd` launcher start and add a provider cloud-init shell part that runs after deferred writes. This preserves the required ordering explicitly instead of depending on cloud-init list merging or systemd startup races.

### Generate a preflight manifest from uploaded assets

Render the expected destination paths from the same Terraform file map used for uploads. The final-stage shell part runs the inline preflight script before enabling the launcher and exits with an actionable message if any expected asset is missing.

## Risks / Trade-offs

- [An older cloud-init image may not support deferred writes] → Validate the supported Ubuntu image cloud-init version in provider deployment tests.
- [A failed fetch still requires a new installation attempt] → Fail before launcher startup with the missing path and rerun guidance.
- [The preflight list could drift from the uploaded files] → Generate it from the same `bootstrap_node_files_by_key` map as the object resources.

## Migration Plan

Deploy the Terraform/cloud-init changes with the normal provider update. Existing deployments are unaffected; a new deployment receives the deferred-fetch sequence. Rollback is a code rollback that restores the original early launcher start and non-deferred entries.

## Open Questions

None.
