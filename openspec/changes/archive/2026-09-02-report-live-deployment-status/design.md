## Context

The listing currently calls the persisted-state-only status path serially. The parent #311 change bounds the canonical status command to five seconds, but library callers must still provide their own deadline.

## Goals / Non-Goals

**Goals:**

- Reuse canonical deployment status resolution.
- Bound the complete set of status probes to five seconds.
- Preserve deterministic output and tolerant directory handling.

**Non-Goals:**

- Changing canonical status semantics, including database identity verification tracked by #309.
- Adding listing-specific timeout configuration.

## Decisions

Derive one five-second context using the timeout defined by the status command and pass it to every deployment status call. Resolve statuses concurrently with one standard-library goroutine per deployment, writing results to distinct slice indexes before sorting. This keeps total latency bounded without a worker-pool abstraction for the small number of launcher-managed deployments expected per workstation.

Call `deploy.Status` rather than the persisted-state-only `deploy.GetStatus` path so each list entry reports the same value as an individual `exasol status` invocation. Preserve the existing per-entry fallback to `not_initialized` when status or preset metadata cannot be read.

Replace the database driver's nested save-and-restore logger swaps with reference-counted suppression. The first overlapping probe saves the original logger and installs the discard logger; only the last probe restores the original. Holding the mutex for the full probe was rejected because it would serialize the network operations this change needs to run concurrently.

## Risks / Trade-offs

- [Many deployment directories create many short-lived goroutines] -> Add a worker limit only if real deployment counts make this measurable.
- [Concurrent probes restore a process-global driver logger out of order] -> Reference-count overlapping suppression and restore only after the last probe.
- [Canonical status can identify the wrong database on a reused port] -> #309 remains responsible for database identity verification.
- [A timed-out or malformed entry falls back to `not_initialized`] -> Preserve current tolerant behavior so one broken directory cannot fail the complete listing.
