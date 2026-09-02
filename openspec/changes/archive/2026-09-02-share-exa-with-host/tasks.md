## 1. VM Copy Support

- [x] 1.1 Add GNU `cp` to the Exasol Local VM image so the existing guest
  command interface can copy directory trees to VirtioFS while preserving
  modes, links, timestamps, and sparse allocation, and cover the copy behavior
  in runner tests

## 2. Host Layout and Migration

- [x] 2.1 Add one host-layout completion marker and a macOS migration
  transaction with reserved staging, atomic publication, conflict detection,
  interrupted-copy recovery, path-rich errors, and focused unit tests
- [x] 2.2 Switch macOS Nano startup to the host-shared `exa` path, retain and
  finalize the guest backup only after durable database readiness, expose it
  through the managed `local/runtime/exa` symlink, and cover fresh, migrated,
  repeated, conflicting, failed, stopped, and destroyed lifecycle behavior
- [x] 2.3 Replace a macOS deployment's guest operating system when the resolved
  runner differs from the one recorded for it, stopping a running VM first,
  recording the runner only after the replacement succeeds, and covering the
  differing, matching, running, and failed cases

## 3. Configuration Semantics

- [x] 3.1 Keep macOS `data_size_gb` as VM runtime-disk sizing, update its
  user-facing description, and cover existing manifest compatibility and
  direct-host behavior

## 4. Cross-Platform Verification

- [x] 4.1 Extend macOS deployment tests to verify complete guest-to-host data
  migration, bidirectional file visibility, committed rows, sparse allocation,
  normal restart, forced VM recovery, conflict handling, retry, and destroy
- [x] 4.2 Extend Windows deployment tests to verify bidirectional `/exa` file
  visibility, committed rows, normal restart, forced Podman-machine recovery,
  adoption without copying into the machine, and deployment-owned data removal

## 5. User Guidance

- [x] 5.1 Document the host-side `/exa` layout, macOS migration and rollback,
  Windows passthrough, VM runtime-disk sizing, and accepted performance
  trade-off, and add the user-visible change to the changelog

## 6. OpenSpec Completion

- [x] 6.1 Verify the proposal, design, specifications, tests, and implementation
  agree, then archive the change and sync every delta into the main specs before
  committing the OpenSpec artifacts
