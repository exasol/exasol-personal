## Why

VM-backed local deployments keep durable Nano data inside a guest disk, which
makes the deployment harder to inspect, back up, and exchange with host tools.
Nano supports an absolute, writable host directory mounted at `/exa`, and the
measured macOS database workload remained correct with acceptable performance,
so local deployments should use that more flexible layout consistently.

## What Changes

- Expose the complete persistent Nano `/exa` tree at `local/runtime/exa` in the
  deployment directory on Linux, macOS, and Windows.
- Mount the deployment's host-side `exa` directory at `/exa` in Nano across all
  supported local platforms.
- Migrate existing macOS `/exa` data from the guest data disk into the host
  deployment directory before starting Nano with the new mount.
- Replace a macOS deployment's guest operating system when the resolved runner
  differs from the one recorded for it, so migration runs against the guest the
  launcher expects.
- Make migration conflict-safe, interruption-safe, and retryable, while
  retaining the guest copy until the host copy has started successfully.
- Treat the host filesystem as the source of database storage capacity while
  retaining the macOS data disk and its existing sizing setting for Podman and
  other VM runtime state.
- Verify host visibility, persistence, restart, recovery, and migration on
  macOS and Windows, with Linux as the existing host-mounted reference.

## Capabilities

### Modified Capabilities

- `exasol-local-deployment`: Make the deployment directory the owner of Nano
  data on every supported local platform, define its lifecycle and visibility,
  and clarify the remaining purpose of macOS VM data-disk sizing.
- `vm-backed-podman-runtime`: Mount macOS Nano data from the managed host share
  and preserve existing data while migrating it from the former guest disk.

## Impact

The change affects local deployment configuration, macOS VM startup, guest
replacement and data migration, shared Podman installation behavior,
deployment destruction,
Windows filesystem-passthrough validation, local lifecycle and migration tests,
documentation, and release notes. The Exasol Local VM runner image must provide
a sparse-preserving copy utility, keep its data disk accessible during
migration, and continue using it for Podman runtime state.
