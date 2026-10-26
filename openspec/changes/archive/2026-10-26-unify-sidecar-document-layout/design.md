## Context

See proposal.md for motivation.

Three facts shape the approach. The catalog entry already holds a `container`
under a name key, so the target shape exists and is in use. The catalog loader
already derives a container name from its key and rejects a name that
contradicts it, so the naming rule exists too. The saved document is read and
written at one boundary, so the container runtime layer below it sees
containers rather than documents.

## Goals / Non-Goals

**Goals:**

- One shape, one set of validation locations, and one naming rule across the
  catalog and the saved document.
- A saved sidecar that reads as one unit under the name it is known by.

**Non-Goals:**

- Changing the catalog file's own layout.
- Changing the supported Container subset, port publishing, or how status
  reports endpoints.
- Changing anything below the document boundary.

## Decisions

**The catalog entry and the saved entry share one type.** `Definition` holds
the `container` a sidecar runs. `Document` keys a map of them by sidecar name,
and the catalog's `Entry` embeds one inline beside its description and
architectures. A catalog entry is then its metadata plus the definition that
enabling saves, so enabling stores one entry under its name instead of
appending to a list. Field access through the embedded type keeps existing
readers of `entry.Container` working, and a saved sidecar that later carries
more than a container grows the shared type once.

**The map key names the container.** The catalog loader already fills an absent
`container.name` from its key and rejects a name that differs. Applying that
same rule to the saved document gives both files one naming rule and leaves the
key as the single place a sidecar is named. A `name` inside `container` stays
accepted so the saved value remains a Kubernetes Container.

**Validation locations follow the key.** Error locations become
`sidecars.<name>.container...`, which is what the catalog already reports.
Reading the saved document wraps its errors with the file name, so an error
still says which file to edit.

**The document hands callers a sorted name order.** A map has no order, so the
document exposes its names sorted and callers iterate through that. Status
without a name already reports in name order, so that output is unchanged.
Applying definitions and recording per-sidecar failures gain a defined order
they previously took from the list.

**The map key replaces the duplicate-name check.** A YAML mapping rejects a
repeated key, so one sidecar name can appear only once and the parser no longer
compares names across a list.

**Applying definitions keeps taking containers.** The apply path receives the
containers it should run, built from the document in name order. The change
stops at the document boundary, so container creation, matching, port
publishing and cleanup are untouched.

## Risks / Trade-offs

[A `sidecars.yaml` written by an earlier development build carries the former
shape] → the launcher reports a parse error naming the file, and enabling the
sidecar again writes the current shape. No released version writes the file.

[A future caller iterates the map directly and gets an arbitrary order] → the
sorted name accessor is the documented way to walk the document, and the
callers this change touches all use it.
