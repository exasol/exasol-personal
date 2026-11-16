## Context

See [proposal.md](proposal.md) for the user problem. Startup migration runs
before deployment-dependent flag registration and command dispatch. It keeps a
legacy deployment in place while the deployment is running, locked, or cannot
be moved. The command resolver and the early flag scan already call the same
precedence function, but named and implicit default selection currently build
only the new managed path. An explicit legacy `--deployment-dir` already works.

## Goals / Non-Goals

**Goals:** Keep early flag registration and command execution on the same
selected path. Preserve the existing selection source for user notices and
structured logging.

**Non-Goals:** Change the startup migration rules or the output format of
`exasol deployments list`.

## Decisions

1. **Resolve both managed locations in the shared precedence function.** A
   helper will check the new path first, then the matching legacy path, and
   return the new path when neither exists. Both `--deployment <name>` and
   implicit default selection will call it. This keeps the pre-Cobra flag
   scan aligned with command execution. Resolving separately in command
   handlers would let dynamic flags be registered from a different deployment.

2. **Treat an occupied new path as the selected path.** An existing new path
   wins even if startup migration also reports a collision with a legacy
   path. This follows the managed-location preference and avoids choosing a
   legacy deployment based on whether the new path is initialized. An
   explicit `--deployment-dir` remains available for either path.

3. **Return path inspection errors.** A failed check of the new or legacy
   path will identify that path and stop resolution. Treating an unreadable
   path as absent could select or initialize a different deployment.

4. **Warn through the resolved-path notice.** Named selection remains
   `named`; implicit default selection remains `default`. When either selects
   a legacy path, its single resolved-path notice will identify that path as
   legacy and use a warning prefix. Structured logs will carry the selected
   path. The `migrated` source remains reserved for an explicit legacy path
   redirected after a successful move. The selection warning is distinct from
   a startup migration warning, which reports why the move did not occur.

5. **Merge listing entries by the same location preference.** Scan both
   managed roots. Add entries from the new root first, then add a legacy
   entry only when its name has no path at the new root. Check the new path
   using the platform's file-system semantics so the list agrees with named
   selection on case-insensitive file systems. Keep the existing concurrent
   status checks, shared timeout, sorting, and JSON fields. A listing entry's
   path identifies when it comes from the legacy root.

## Risks / Trade-offs

- Checking managed paths adds file-system reads to every named or implicit
  default selection. Keep the checks limited to the two candidate paths and
  return errors with the path that could not be inspected.
- A legacy default selected by `install` may already contain initialized
  state. Keep the existing preset compatibility gate on the resolved path and
  verify this case through the launcher.
- A collision leaves a legacy directory that the list does not show. The
  startup migration warning names both paths, and `--deployment-dir` can
  select the legacy path directly.

## Migration Plan

The change adds a read-time fallback and does not alter persisted deployment
data. Startup migration continues to move a kept deployment after it becomes
available. A rollback restores the previous path selection behavior without
converting data.
