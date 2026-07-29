## 1. Contract and runtime model

- [x] 1.1 Implement and test local-vm v2 configuration, hook, state, forwarding, and caller-owned-path contracts.
- [x] 1.2 Remove Nano, Exasol initialization, SLC, readiness, and Windows runtime behavior from local-vm.
- [x] 1.3 Add runtime-only `WorkloadSpec`, `RuntimeAdapter`, `RuntimeStatus`, and `RuntimeCapabilities`.
- [x] 1.4 Add deterministic deployment-scoped workload names and labels.

## 2. Declarative workload

- [x] 2.1 Render a private Kube Pod preserving image digest/no-pull, `/exa`, 512 MiB `/dev/shm`, unlimited PIDs, unmasked proc, restart, port, arguments, and SLCs.
- [x] 2.2 Implement idempotent workload helper `apply`, `down`, `status`, and `logs` modes with `podman kube play --replace`.
- [x] 2.3 Package architecture-specific Nano image inputs in Personal and verify checksums.
- [ ] 2.4 Complete the Podman Kube compatibility spike and set the tested minimum version.

## 3. Platform adapters

- [x] 3.1 Implement macOS VM config/hook/share generation and SSH helper operations.
- [x] 3.2 Preserve macOS SLC, VM shell, container shell, resources, forwarding, and SQL readiness.
- [x] 3.3 Implement Windows WSL2 Podman detection, optional Winget setup/upgrade, machine reuse/init, path conversion, lifecycle, and diagnostics.
- [x] 3.4 Support Windows amd64 SLC image mounts while rejecting unsupported resource, storage-size, VM-shell, and container-shell capabilities.
- [x] 3.5 Add an explicit unimplemented Linux adapter seam.
- [x] 3.6 Remove every v1 runner invocation and route local lifecycle, shell, status, and diagnostics exclusively through v2 adapters.

## 4. Persistence and migration

- [x] 4.1 Use Personal-owned host-directory storage on both platforms and virtiofs on macOS without a parallel adapter state file.
- [x] 4.2 Remove local `dataSizeGB` from new presets/configuration while parsing legacy values as ignored.
- [x] 4.3 Implement verified, atomic legacy macOS `/exa` migration in the v2 boot hook using the data directory as its only completion marker.
- [x] 4.4 Make explicit deployment destruction the only workload path that removes `/exa`.
- [x] 4.5 Preserve the existing launcher state and compatibility marker during migration.

## 5. Build and release integration

- [x] 5.1 Add development-only `RUNNER_PATH` embedding and release-build rejection.
- [x] 5.1a Add explicit macOS and Windows validation builds and verify OCI image digest, blobs, entrypoint, and architecture.
- [x] 5.1b Fetch and embed repository-pinned Nano images in trusted release CI while allowing synthetic placeholders in test builds.
- [ ] 5.2 Validate Personal against a manually built local-vm artifact before freezing schemas.
- [ ] 5.3 Publish local-vm v2.0.0 only after Personal integration passes, then pin URL/version/checksum.
- [x] 5.4 Retain immutable v1 artifacts for older Personal releases.

## 6. Verification

- [x] 6.1 Cover reconstruction, disposable generated assets, manifest invariants, idempotency, and concurrent deployment isolation.
- [x] 6.2 Cover Windows prerequisite/machine/path fakes, SLC pulls, and unsupported shell/resource capability failures.
- [ ] 6.3 Complete hardware coverage for v2 guest migration, SQL failure, retry, and virtiofs persistence.
- [ ] 6.4 Run macOS and Windows integration scenarios, persistence, SLC, packaging, signing, notarization, and full project suites.
- [ ] 6.5 Update permanent specifications, archive the change, and add changelog entries only after end-to-end validation.
