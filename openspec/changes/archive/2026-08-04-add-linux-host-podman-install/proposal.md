## Why

The Linux host runtime currently loads the Nano artifact but starts a hard-coded image without the persistent storage and runtime options required for a usable local database. A small native Podman installation boundary is needed before the broader local-runtime refactor can move container handling into Exasol Personal.

## What Changes

- Add a runtime-neutral configuration boundary for local runtime settings and Podman installation settings while keeping VM sizing isolated in `VMConfig`.
- Start the Nano database through Podman on `LinuxHostRuntime` with a persistent `/exa` mount, a configurable host DB port, fixed Nano container settings, and first-start initialization parameters.
- Reuse an already-running deployment container and make stop and destroy cleanup idempotent.
- Derive the runnable image reference from `podman load` instead of hard-coding a Nano release tag.
- Exclude CPU, memory, and storage limits for plain host containers.
- Exclude image caching and cleanup, migration and recovery, SLC support, version-check configuration, diagnostics, readiness reporting, and runtimes other than `LinuxHostRuntime`.

## Capabilities

### New Capabilities

- `linux-host-podman-install`: Starting and removing a persistent Exasol Nano Podman container directly on a Linux host.

### Modified Capabilities

None.

## Impact

- Local runtime and local installation interfaces.
- Linux host runtime configuration mapping and Podman command execution.
- Unit tests for local runtime paths, configuration parsing, and container lifecycle behavior.
- No new user-facing configuration fields or external dependencies.
