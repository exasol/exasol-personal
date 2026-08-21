## Why

Each deployment currently creates a redundant local tag for the same Nano image after loading its archive. Podman already reports a runnable image name or ID, so the installer can use that reference directly and avoid accumulating deployment-specific aliases.

## What Changes

- Start Nano with the exact image reference reported by `podman load`.
- Stop creating a deployment-specific `localhost/<container>:latest` tag.
- Keep image loading and loaded-reference validation unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-host-podman-install`: Require the shared Podman installer to use the loaded Nano image reference directly without creating a deployment-specific alias.

## Impact

The shared Podman installation path and its unit tests change for Linux-host and macOS VM-backed local deployments. No public API, configuration, or dependency changes are required.
