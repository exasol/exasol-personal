# Local Deployment

This document describes, at a high level, what happens when a user runs `exasol install local`.

Local deployment is a special launcher mode for Apple Silicon macOS. It preserves the normal deployment-directory workflow, but it does not provision cloud infrastructure and it does not route lifecycle operations through OpenTofu or SSH-based host management.

## Local Install Flow

During a local install, the launcher:

1. validates that the host and selected presets support local mode
2. initializes the deployment directory and records launcher-managed state
3. reuses the cached local runtime payload when present, or extracts the embedded payload baseline and guest boot assets into the local cache
4. prepares deployment-scoped local runtime state, ports, logs, and persistent data
5. starts the local guest runtime
6. installs Exasol inside that guest through the selected local-compatible installation preset
7. waits until the database is ready for connections
8. writes deployment metadata, secrets, and connection instructions for later commands such as `info`, `connect`, `start`, `stop`, and `destroy`

## Operational Model

The deployment directory remains the source of truth for the local deployment, just as it does for cloud-backed deployments. The main difference is that the backend behind that directory is local runtime management instead of cloud provisioning.

Local deployment artifacts and runtime state are owned by the deployment directory, which allows multiple local deployments to remain isolated from each other.

The current local payload model is embedded-first. The launcher does not fetch the local runtime payload from a remote HTTP endpoint during local startup; it relies on the embedded payload baseline and the local cache.

## See Also

- [Architecture](architecture.md)
- [Deployment directory compatibility](deployment_compatibility.md)
- [Local deployment design](../openspec/changes/add-local-deployment/design.md)
