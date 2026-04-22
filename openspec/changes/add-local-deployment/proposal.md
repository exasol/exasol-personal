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
- Add versioned Linux ExaNano `.run` payload download, verification, and caching from a product-owned HTTP location.
- Add local-safe deployment artifacts and command behavior for `info`, `connect`, `status`, `start`, `stop`, and `destroy`.
- Keep unsupported shell-style commands honest for local deployments when there is no real equivalent.

## Impact

Impacted capability areas:

- deployment lifecycle
- preset resolution and validation
- local runtime management
- deployment artifacts and connection rendering
- macOS arm64 build and release flow

Impacted code areas:

- `internal/deploy`
- `internal/presets`
- `internal/config`
- new `internal/localruntime` package family
- selected command handlers in `cmd/exasol`
- build, installer, and release configuration for macOS arm64
