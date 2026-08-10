## Context

The existing Linux host runtime delegates to a minimal Podman installer behind a temporary backend. The macOS VM runner and Linux host installer consume overlapping launcher state through different paths, while the current shell reference script contains additional Nano version-check, SLC, recovery, and migration behavior. The custom-SLC change is active independently, so this change consumes its resulting state without taking ownership of its user-facing specification.

## Goals / Non-Goals

**Goals:**

- Make runtime selection and launcher-to-install translation single-source policies.
- Keep Podman containers disposable while preserving and recovering deployment-owned data.
- Match the shell reference's observable Nano startup behavior with deterministic, unit-testable Podman commands.
- Preserve macOS VM behavior and diagnostics compatibility.

**Non-Goals:**

- Add Podman-machine support on Windows.
- Port VM init/output JSON, guest syslog collection, or a background container-log follower.
- Add Nano image checksum/load caching or stale Nano image pruning.
- Migrate manifests created by the temporary test-only `local-host` backend.

## Decisions

1. Keep a runtime-neutral installation start contract.

   `StartConfig` carries the published database port, persistent data path, first-start parameters, version-check settings, and optional SLC declarations. Podman owns Nano constants such as internal port, shared memory, PID limit, security options, restart policy, and SELinux relabeling. This keeps policy shared without leaking Podman flags into the macOS runner path.

2. Distinguish unknown SLC state from an authoritative empty set.

   A nil SLC slice means the caller is unaware of SLC state and disables pruning. A non-nil empty slice means no SLC is referenced and permits managed-image pruning. Collapsing both states would risk deleting images for older or partial callers.

3. Query exact container identity for lifecycle state.

   Status and migration inspection target the deployment-specific container name rather than filtering broad Podman listings. A missing container maps to stopped; malformed or failed inspection remains an error. The deployment layer maps this to its stable runtime status representation.

4. Treat diagnostics as best-effort secondary output.

   Failures in image materialization, recreation, or startup trigger `podman info`, `podman ps -a`, exact-container inspection, and logs. Diagnostic failures are logged but never replace the causal error. The VM's continuous log follower remains VM-specific.

5. Materialize SLCs before container recreation.

   Existing images are reused, missing official images are pulled, and custom packages are imported with a management label. Official pull failure is fatal because the requested runtime cannot be constructed; custom package failures only mark that SLC unavailable. The installer atomically rewrites a status file and mounts only available images using Podman image mounts.

6. Prune by ownership and normalized exact references.

   Official images are eligible only when their normalized repository is exactly the official SLC repository; custom images require the import label. Registry aliases and implicit `latest` are normalized before reference comparison. Untagged and unrelated images are never removed, and all cleanup is nonfatal.

7. Recover partial initialization by quarantine.

   An initial-create marker identifies a startup that may have left inconsistent `/exa` state. Recovery removes the disposable container, renames the data directory to a timestamped sibling, and creates an empty replacement. Without `exasol.conf`, only known stale TLS files are removed; initialized databases retain TLS and omit first-start parameters.

8. Migrate legacy overlay data through sibling staging.

   A legacy exact container without an `/exa` mount is stopped but retained while `podman cp` copies its data into a sibling staging directory. Migration refuses a populated persistent destination. The staged directory is atomically renamed into place before the source container is removed, leaving either source or staged data recoverable on every failure path.

9. Select the runtime through one platform policy.

   macOS ARM64 selects the VM runtime; Linux AMD64 and ARM64 select host Podman; all other pairs are unsupported. Backend creation, reconciliation, SLC operations, connect diagnostics, and `diag local` call that policy. VM sizing flags, validation, and memory notices are gated by the selected platform rather than by temporary backend names.

## Risks / Trade-offs

- Podman CLI and inspect output can vary across versions -> isolate parsers, request stable formatted fields where possible, and cover command/output contracts with a fake executable.
- Atomic directory replacement requires source and destination siblings on one filesystem -> always stage beside the deployment data directory.
- Best-effort pruning can leave stale managed images -> log failures and retry on later SLC-aware starts rather than failing database availability.
- A live migration failure may leave a stopped legacy container -> retain it deliberately and return recovery instructions rather than deleting the only complete copy.
- Linux health can only observe the published SQL socket, not internal Nano readiness details -> preserve the existing bounded database readiness check at the deployment layer.

## Migration Plan

1. Introduce portable interfaces while both runtimes compile and pass focused tests.
2. Add Podman lifecycle parity and migration before exposing Linux through `local`.
3. Centralize platform selection, remove the temporary backend, and retain existing deployment JSON keys.
4. Validate and smoke-test a default Linux deployment before archiving the change.

Rollback consists of reverting the promotion commit while retaining the backward-compatible installer and runtime changes. Overlay migration is one-way after successful atomic installation, but leaves data in the deployment-owned layout used by both the completed and minimal Podman implementations.
