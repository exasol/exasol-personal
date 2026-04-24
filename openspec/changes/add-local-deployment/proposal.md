# Proposal: Add Local Deployment Mode

## Why

Exasol Personal currently assumes cloud-backed infrastructure for first-class deployments. That makes local development, demos, and fast feedback loops heavier than they need to be.

We need a built-in `local` deployment mode that preserves the existing deployment-directory lifecycle and CLI shape while running Exasol locally on Apple Silicon macOS.

The key constraint is that the ExaNano runtime already exists as Linux `.run` artifacts. The missing capability is the host-side virtualization and orchestration layer on macOS. That host-side layer must live inside the `exasol` launcher rather than depending on a separate native wrapper.

## What Changes

- Add a built-in `local` infrastructure preset.
- Add an explicit infrastructure/installation compatibility model so only valid preset pairs are allowed.
- Add a dedicated local deployment backend rather than routing local through Tofu, SSH, or cloud power-state helpers.
- Add a deployment-scoped local runtime model under the deployment directory.
- Add a launcher-owned v1 local credential contract with fixed defaults (`sys` / `exasol`).
- Add versioned Linux ExaNano `.run` payload download, verification, and caching from a product-owned HTTP location.
- Add local-safe deployment artifacts and command behavior for `info`, `connect`, `status`, `start`, `stop`, and `destroy`.
- Finish the common `deployment.json` contract so local mode does not depend on a second local-only deployment-info schema.
- Add launcher-owned local VM sizing configuration instead of relying only on fixed code constants.
- Keep the v1 local guest scope limited to the database and admin UI; notebook and UDF-related extras remain out of scope.
- Keep unsupported shell-style commands honest for local deployments when there is no real equivalent.

## Impact

Impacted capability areas:

- deployment lifecycle
- preset resolution and validation
- local runtime management
- deployment artifacts and connection rendering
- credential management
- local runtime sizing and guest-scope policy
- macOS arm64 build and release flow

Impacted code areas:

- `internal/deploy`
- `internal/presets`
- `internal/config`
- new `internal/localruntime` package family
- selected command handlers in `cmd/exasol`
- build, installer, and release configuration for macOS arm64
