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
- backend-specific deployment-directory interactions such as diagnostics, shell behavior, and
  shell execution

Backend boundary policy:

- commands that operate on an initialized deployment directory resolve the backend first
- backend-specific behavior stays behind backend interfaces
- backend-private files and schemas are not inspected directly from command code
- backends return data and operations; common launcher code formats text and JSON output
- backend validation is environment-oriented rather than host-platform-specific in the interface

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

Compatibility is not only a validation rule. CLI help and preset-listing output should surface the
embedded compatibility matrix and should explicitly call out that `local` is a special built-in
preset rather than just another cloud provider row.

### Common deployment info contract

`deployment.json` should be a single launcher-facing contract across backends.

- common fields stay stable across backends
- backend-specific sections use optional fields
- launcher code reads one normalized schema instead of branching on backend-private file shapes
- local deployments do not get a second local-only wrapper schema once the transition is complete

Compatibility migration policy:

- local compatibility shims may exist while older artifacts are still supported
- new code paths should read and write the common deployment-info contract directly
- compatibility helpers must not become a second steady-state abstraction layer

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

### Local credential policy

V1 local mode uses a launcher-owned fixed local credential contract.

- the launcher owns the credential interface for local deployments
- the default local SQL credentials are fixed to `sys` / `exasol`
- if the Linux `.run` payload requires explicit credential injection, that handoff must be modeled as
  an explicit launcher-to-guest contract
- connection instructions and `secrets.json` reflect that launcher-owned local credential contract
- credential literals should be centralized in code rather than duplicated across the local backend

### Local VM sizing policy

Local VM sizing must be launcher-owned configuration rather than unexplained code constants.

- CPU, memory, and persistent layer-disk sizing have documented defaults
- sizing may be sourced from preset defaults, launcher-managed variables, or both
- the runtime consumes normalized sizing inputs instead of encoding product policy directly inside VM
  bootstrap helpers

### Local guest scope for v1

The v1 local guest scope is intentionally narrow.

- required surface:
  - database runtime
  - admin UI
- excluded from v1:
  - Jupyter
  - Voila
  - UDF/runtime-stack provisioning extras

This keeps the first launcher-owned VM path focused on running the Linux `.run` payload reliably
before wider guest-side feature parity is considered.

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
- if credential ownership is not made explicit, local mode will drift into unsafe or misleading
  defaults
- if VM sizing remains hardcoded, product policy will leak into low-level runtime code and become
  difficult to evolve
- if compatibility validation lands late, invalid preset combinations will leak across the codebase
- if control transport details leak outside the local runtime subsystem, future changes will be expensive
