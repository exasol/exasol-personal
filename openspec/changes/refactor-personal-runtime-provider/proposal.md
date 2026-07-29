## Why

The embedded local VM runner currently owns Exasol Nano image selection, database
container creation, persistence, SLC mounts, and version checks. This couples the
product workload to one macOS VM implementation and prevents Personal from producing
the same local workload on Windows.

## What Changes

- Make the macOS local VM runner a generic, versioned, configuration-driven VM
  provider that owns only VM, SSH, share, forwarding, and boot-hook mechanics.
- Reconstruct an in-memory Personal workload specification for every command and
  reconcile it through platform adapters.
- Generate a private Podman Kube manifest that preserves Nano security, shared-memory,
  persistence, restart, port, version-check, and SLC behavior.
- Add direct Podman lifecycle and prerequisite support for Windows with WSL2 while
  retaining a future Linux adapter seam.
- Move persistent local data into a Personal-owned directory and migrate legacy data
  from the preserved v2 guest disk through a one-off boot hook without executing a v1
  runner or extending launcher state.
- Fetch and embed digest-pinned Nano OCI archives for macOS arm64 and Windows amd64 in
  trusted packaging CI while retaining synthetic placeholders for ordinary tests.
- Remove user-visible local data sizing while continuing to parse the legacy setting
  as deprecated and ignored.
- Permit development builds to embed an explicitly supplied local-vm v2 binary, while
  rejecting that override in release builds.

## Capabilities

### New Capabilities

- `local-runtime-adapters`: reconstruct and reconcile a Personal-owned local workload
  through platform-specific runtime adapters.
- `local-workload-persistence`: preserve local database data independently of
  replaceable VM and Podman runtime state.
- `local-runtime-migration`: migrate legacy macOS deployments without unverified
  deletion or persistent migration checkpoints.

### Modified Capabilities

- `exasol-local-deployment`: support macOS/arm64 and Windows/amd64 WSL2 from one
  Personal-owned workload definition.
- `local-runtime-boundaries`: make Personal the workload owner and local-vm a generic
  macOS VM provider.
- `local-reachability-diagnostics`: combine product configuration, live adapter state,
  forwarding, and SQL readiness.

## Impact

- `internal/runtimeadapter`, `internal/deploy`: runtime-only workload types, platform
  adapters, lifecycle selection, capabilities, and diagnostics.
- Remove `internal/localruntime`; `internal/runtimeadapter` is the only local-vm
  invocation layer.
- `internal/config`, `internal/presets`: preserve existing deployment state and
  deprecated local data-size parsing without new local-runtime fields.
- `assets`: private workload helper, Kube assets, per-platform Nano image input, and
  local-vm development embedding.
- `exasol-local-vm`: companion generic v2 configuration/hook/state contract; released
  only after Personal validation.
