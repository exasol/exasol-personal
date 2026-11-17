## Why

On Linux and Windows, `container_engine` `auto` selects Podman when both Podman and Docker are installed. Users who have Docker installed expect it to be used, so `auto` should prefer an installed Docker over Podman.

## What Changes

- `auto` selects an installed Docker before an installed Podman on Linux and Windows.
- With neither engine installed, Linux still uses Podman, and Windows still offers Podman first and installs it with `--auto-approve`.
- Engine lists in help and errors follow the new order of preference.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `docker-host-runtime`: `auto` prefers Docker over Podman when both are installed.

## Impact

Affects engine selection for new Linux and Windows local deployments, the CLI reference, user documentation, and the changelog. Deployments that already recorded their engine keep it.
