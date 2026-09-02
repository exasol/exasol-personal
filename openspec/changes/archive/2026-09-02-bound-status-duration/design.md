## Context

The status command passes its unbounded command context through deployment locking, local runtime inspection, and the database readiness probe. The database connection honors context cancellation, but the command currently supplies no deadline.

## Goals / Non-Goals

**Goals:**

- Bound the complete status operation by default.
- Let callers select a different positive number of seconds.
- Reuse Go context cancellation and Cobra integer parsing.

**Non-Goals:**

- Changing status values or database readiness semantics.
- Changing timeouts for other commands.

## Decisions

Register a `--timeout` integer flag with a five-second default. Validate that the number of seconds is positive and representable as a Go duration, derive a timeout context from the command context, and pass it to the existing safe or unsafe status path. Applying the deadline at the command boundary covers lock acquisition, runtime inspection, and database probing without adding timeout handling to each layer.

The option remains command-specific instead of reusing the interactive connection timeout because a quick observational command has a different latency contract.

## Risks / Trade-offs

- [A healthy but slow deployment exceeds five seconds] -> Callers can increase `--timeout`.
- [A dependency does not honor context cancellation] -> Existing database and process boundaries already consume the context; tests verify the command-level bound where practical.
