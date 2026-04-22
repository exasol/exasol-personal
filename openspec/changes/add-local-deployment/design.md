# Design: Add Local Deployment Mode

## Overview

The local deployment feature should preserve the existing user-facing launcher model:

- `exasol install local`
- `exasol init local && exasol deploy`
- deployment-directory state and compatibility checks

But it must not pretend local behaves like cloud infrastructure. The current cloud path depends on:

- OpenTofu provisioning
- SSH-oriented node operations
- cloud power-state actions

That path is not suitable for local execution.

## Core Design

### Deployment backend split

Refactor lifecycle execution around a backend abstraction:

- `tofuBackend` for current cloud behavior
- `localBackend` for built-in macOS local behavior

`internal/deploy` keeps ownership of:

- workflow state
- locking
- deployment-directory compatibility
- high-level command sequencing

Backends own:

- side effects
- environment-specific lifecycle operations
- deployment artifact generation

### Preset compatibility model

Infrastructure and installation presets are not freely composable.

Use a directional compatibility model:

- infrastructure presets declare tags they provide
- installation presets declare tags they require

The launcher validates the pair before mutating deployment state.

Examples:

- valid:
  - `aws ubuntu`
  - `local nano`
- invalid:
  - `aws nano`
  - `local ubuntu`

### Local runtime isolation

All local runtime state must be deployment-scoped under:

```text
<deploymentDir>/local-runtime/
```

This includes:

- runtime config
- logs
- control state
- persistent data
- local runtime state file

Multiple deployment directories must run concurrently without sharing these paths.

### Host-side virtualization

The host-side virtualization layer is built into `exasol`.

Implementation constraints:

- Apple Silicon macOS only for v1
- small helper libraries are acceptable if they stay thin
- `vz`-style wrappers are acceptable
- heavyweight virtualization stacks are not

All virtualization-specific code should be isolated behind `internal/localruntime/vm`.

### Linux ExaNano payload delivery

Linux ExaNano `.run` payloads are external versioned artifacts, not embedded blobs or native macOS runtime dependencies.

The launcher should:

- resolve a pinned Linux `.run` payload version from a product-owned HTTP location
- download the runnable `.run` payload and any minimal guest boot assets needed to start the Linux VM
- verify checksums
- cache them in an Exasol-owned data directory
- boot the guest through the launcher-owned virtualization layer
- invoke the selected `.run` payload inside the guest during bootstrap
- persist the selected payload identity into deployment-owned local runtime state

Supporting boot assets such as kernel or initrd may be published alongside the database payload, but the database runtime contract remains the Linux `.run` artifact executed inside the guest. The launcher must not depend on an ExaNano macOS host binary to run local mode.

### Control channel

For v1, use a mounted control socket/file bridge between host and guest.

Reasoning:

- it aligns with ExaNano's current control model
- it is simpler to debug
- it is simpler to fake in tests
- it has lower initial complexity than vsock

The abstraction should still keep transport details localized so vsock can replace it later if needed.

## Recommended Package Boundaries

- `cmd/exasol`
  - CLI only

- `internal/deploy`
  - backend resolution
  - workflow state
  - command sequencing

- `internal/localruntime`
  - local runtime controller

- `internal/localruntime/assets`
  - payload metadata, download, verification, cache

- `internal/localruntime/config`
  - runtime root layout, config rendering, ports

- `internal/localruntime/state`
  - deployment-owned local runtime state

- `internal/localruntime/vm`
  - virtualization interface and macOS driver

- `internal/tofu`
  - unchanged scope, cloud-only backend

## Delivery Strategy

Recommended implementation order:

1. split deployment backends
2. add preset compatibility validation
3. finish `localCommand` support
4. add local runtime state/config packages
5. add payload manager
6. add VM abstraction and darwin/arm64 driver
7. add mounted control bridge
8. implement `localBackend`
9. expose the `local` preset publicly
10. update build, installer, and release flow

## Main Risks

- macOS arm64 build will likely need different build settings than the current generic launcher path
- deployment artifacts are currently cloud-shaped and need to become local-safe
- if compatibility validation lands late, invalid preset combinations will leak across the codebase
- if control transport details leak outside the local runtime subsystem, future changes will be expensive
