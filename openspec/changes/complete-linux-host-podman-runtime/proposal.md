## Why

Linux host Podman support currently implements only a minimal persistent Nano lifecycle behind a temporary backend. Promoting it to the standard local deployment requires the same configuration, SLC delivery, recovery, migration, status, and health guarantees that users rely on from the existing local runtime.

## What Changes

- Promote the Linux host runtime to the standard `local` backend on Linux AMD64 and ARM64 while retaining the VM runtime on macOS Apple Silicon.
- Make VM sizing configuration and validation macOS-only; Linux containers use unrestricted host resources and expose only common local settings.
- Simplify the runtime-to-install start contract and add portable Nano version-check and SLC configuration.
- Report exact deployment-container status, health, endpoints, and actionable Podman diagnostics.
- Materialize official and custom SLC images, publish availability status, mount available images, and prune only unreferenced managed SLC images.
- Recover interrupted initial database creation without reusing partial state and preserve valid existing TLS material.
- Migrate legacy overlay-backed `/exa` data into deployment-owned persistent storage without risking either source or destination data.
- Remove the temporary `local-host` backend and document Podman as the Linux prerequisite for local deployments.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-host-podman-install`: Complete the Podman installation lifecycle with portable startup options, status, diagnostics, SLC delivery and pruning, interrupted-create recovery, and overlay-data migration.
- `exasol-local-deployment`: Support the standard local deployment on Linux hosts, including platform-specific configuration, lifecycle status, endpoint recovery, health checks, and shell behavior.
- `local-runtime-boundaries`: Select the platform runtime centrally and translate shared launcher state into runtime-neutral installation settings.

## Impact

The change affects the local install and runtime interfaces, Podman command execution, local backend selection and validation, SLC startup integration, diagnostics, deployment metadata reconciliation, tests, the local preset, README prerequisites, and user-visible release notes. Windows remains unsupported and macOS VM behavior remains compatible.
