## Context

The Windows host preparer currently treats rootful mode as a prerequisite: it initializes new machines with `--rootful`, inspects existing machines' mode, and requests approval to stop and convert rootless machines. The IPv4 database-port binding introduced in `2801be90fece2531b5fb67ea23a448421cf7c7a7` fixes the Windows/WSL forwarding failure without privileged container operation. See `proposal.md` for motivation and the specification delta for the corrected behavior.

## Goals / Non-Goals

**Goals:**

- Keep default-machine discovery, initialization, and start retry-safe.
- Preserve existing machines and their configured rootful or rootless mode.
- Remove mode-specific commands, approval requests, implementation branches, and tests.

**Non-Goals:**

- Change Podman installation or its existing approval flow.
- Convert existing rootful machines back to rootless.
- Change the IPv4 binding workaround or Windows machine provider selection.

## Decisions

1. Treat machine mode as irrelevant to readiness.

   Readiness depends only on whether the default machine exists and is running. The preparer will not inspect `Rootful`, avoiding both version-sensitive output parsing and mutation of a shared Podman machine. Retaining mode inspection without acting on it was rejected because it adds failure modes without affecting a launcher decision.

2. Use Podman machine inspection for discovery and state.

   `podman machine inspect --format {{.Name}} podman-machine-default` is the machine-readable interface for identifying the default machine and avoids depending on list presentation. Passing the default name explicitly keeps the target clear. Empty inspection output means the default machine is absent; once present, its state is obtained through the same inspection interface.

3. Let Podman select the mode for newly initialized machines.

   Initialization retains the fixed 40 GB disk size but omits `--rootful`, which respects Podman's platform default and user configuration. Adding an explicit `--rootless` was rejected because the launcher does not require either mode and should avoid unnecessary policy.

4. Remove conversion-only approval vocabulary and tests.

   The privileged-runtime host-change kind has no remaining producer after conversion is removed, so keeping it would expose a dead internal concept. Tests will cover mode-independent creation, starting, and no-op readiness while deleting scenarios for approval and conversion behavior that no longer exists.

## Risks / Trade-offs

- A future Podman or Nano change could reintroduce a rootless-only incompatibility → keep Windows lifecycle coverage and diagnose the concrete runtime failure before adding host mode policy.
- Existing rootful machines remain rootful → this is intentional compatibility; the launcher avoids mutating a shared host resource when either mode works.

## Migration Plan

Ship the simplified preparation with the corrected specification and README. Existing deployments require no migration because machine mode and deployment state are unchanged. Rollback restores mode enforcement but could again prompt users to convert a functioning rootless machine.
