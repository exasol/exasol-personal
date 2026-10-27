## Context

See proposal.md for motivation.

Saved definitions in `sidecars.yaml` are authoritative once a sidecar is
enabled. Start reconciles the containers of every saved entry and removes owned
containers that no saved entry names. Disable relies on that by deleting the
entry and applying the remaining set.

## Goals / Non-Goals

**Goals:**

- A disabled sidecar keeps every saved field, so re-enabling restores the
  user's configuration exactly.
- One explicit command form that returns a sidecar to its catalog template.

**Non-Goals:**

- Changing how a running sidecar is stopped or how its resources are cleaned
  up.

## Decisions

**`enabled` sits on the saved definition, beside `container`.** The field is a
property of a deployment's entry, in the same place as `endpoints`, so a user
reading `sidecars.yaml` sees the state next to the definition it governs. The
`Definition` type carries it as an optional boolean placed first, so it is the
first key under each entry. Catalog validation rejects the field, because a
catalog entry describes a template and enablement belongs to a deployment.

**Enable writes `enabled: true` explicitly.** Every entry the launcher writes
shows its state, so a user editing the file finds the field to change. An entry
without the field counts as enabled, which keeps hand-written entries working.

**Saved and enabled are separate predicates.** Status, list, and
reconciliation ask whether an entry is enabled. Enable, the unknown-name check,
and status without a name ask whether an entry is saved. `Document.Containers`
returns the containers of enabled entries only, so start and apply remove a
disabled sidecar's owned container through the existing undesired-container
path. Failures recorded against a whole document name enabled entries only.

**Removal is a flag on disable.** `exasol sidecar disable <name> --remove`
shares disable's cleanup, failure recording, and deferred behavior on a stopped
deployment; the only difference is that the entry is deleted instead of marked
disabled. The deploy package exposes it as `RemoveSidecar`.

**Repeated disable keeps the existing `nothingToRemove` result.** A disable
whose target is already disabled, or a removal whose target is already absent,
reports `nothingToRemove: true` unless an unresolved cleanup failure is recorded
for that name, in which case it retries cleanup. A name the deployment has not
saved is treated as already removed, which also lets a cleanup retry name a
sidecar whose entry is gone.

## Risks / Trade-offs

[A disabled entry stays in `sidecars.yaml` and is still validated] → an invalid
edit to a disabled entry fails every sidecar operation, as it does for an
enabled one. The validation error names the offending field.
