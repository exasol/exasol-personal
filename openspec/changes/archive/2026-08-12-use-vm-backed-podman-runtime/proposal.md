## Why

The macOS local runtime still delegates database-container ownership to the VM runner, while the Linux local runtime uses the shared Podman installation directly. Moving container ownership into exasol-personal makes local installation behavior consistent and keeps VM transport details out of deployment workflows.

## What Changes

- **BREAKING** Require the VM-only v2 runner contract for macOS local deployments.
- Start the macOS VM with labeled service forwards, then run the shared Podman installation inside the VM through `launcher run`.
- Make Podman installation commands and filesystem operations execute through a runtime-provided environment.
- Represent staged artifacts with both host and runtime paths and load images with `podman load -i <runtime-path>`.
- Keep Linux host behavior unchanged through a direct execution environment and identity path mapping.
- Route VM and container shells through the local runtime instead of reading SSH details from launcher state.
- Remove legacy macOS runner state and database payloads when a deployment adopts the VM-only runner.

## Capabilities

### New Capabilities
- `vm-backed-podman-runtime`: Running the shared local Podman installation inside the macOS VM through a transport-neutral launcher contract.

### Modified Capabilities
- `linux-host-podman-install`: Generalize the existing Podman installation behavior to runtime-provided command, filesystem, and artifact paths while preserving Linux host semantics.
- `local-runtime-boundaries`: Delegate guest execution, path mapping, shell access, and endpoint resolution to the selected runtime without exposing SSH transport.
- `exasol-local-deployment`: Make macOS start the VM before installing the database container and derive connection endpoints from labeled VM forwards.

## Impact

This affects the local runtime interfaces, Podman installation implementation, macOS runtime lifecycle, shell delegation, local deployment metadata, migration cleanup, tests, and architecture documentation. It depends on the VM-only v2 exasol-local-vm launcher but does not change runner release resources or their pinned version in this change.
