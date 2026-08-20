## Why

Upgrading a macOS local deployment created by v2.2.0 can overwrite its already-persistent `/exa` data because legacy container-name adoption incorrectly forces overlay migration. Stale deployment tests also assert removed runner and SSH artifacts and can leak a running VM when an assertion fails. The shared Podman refactor dropped the image-storage synchronization previously performed by `init-db.sh`, and Nano does not durably commit all startup files before reporting database readiness.

## What Changes

- Detect the source and destination of the existing container's `/exa` mount and skip migration when it already uses the intended persistent data directory.
- Preserve staged migration for overlay-backed data and data mounted from a different location.
- Remove the legacy-name-based forced migration mode.
- Test the endpoint-based local deployment contract and ensure standalone deployment and chaos tests always destroy their temporary VMs.
- Synchronize local runtime storage after image preparation on Linux and macOS, and explicitly work around Nano's startup durability defect after database readiness.
- Remove automatic Podman repair/reset experiments and avoid marker-based Podman recovery.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `exasol-local-deployment`: Require legacy-name adoption to preserve data that is already mounted from the managed persistent data directory and require successful local startup to cross explicit durability boundaries.

## Impact

The shared local installation, Linux and macOS runtimes, their Go unit tests, and standalone macOS deployment and chaos tests are affected. The Terraform formatting task also uses the repository's OpenPGP build tag so the rewritten commits can be validated consistently. No public CLI, configuration, runner CLI, or deployment file format changes.
