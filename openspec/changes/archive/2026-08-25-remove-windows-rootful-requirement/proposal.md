## Why

Windows local deployments work with Podman's default rootless machine once the database port is explicitly bound to IPv4, so requiring rootful mode is unnecessary. The current requirement disrupts existing rootless machines, introduces an avoidable approval step, and adds preparation code and tests for a host change the launcher does not need.

## What Changes

- Accept the default Windows Podman machine in either rootless or rootful mode.
- Initialize a missing default machine without overriding Podman's default mode, and start an existing stopped machine without changing its mode.
- Remove the privileged-runtime host-change kind, rootless-to-rootful conversion logic and prompt, and test assertions that only cover that conversion.
- Correct the Windows prerequisites and release documentation so it no longer promises or requires rootful operation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `windows-host-runtime-environment`: Make default-machine readiness independent of Podman's rootful/rootless mode and remove mode conversion from host preparation.

## Impact

The change affects the Windows host environment preparer, command-layer host-change presentation, focused Go tests, Windows lifecycle integration coverage, the Windows host runtime specification, and user-facing documentation. Windows Podman installation approval remains unchanged, existing rootful machines continue to work, and no dependency or persisted deployment format changes.
