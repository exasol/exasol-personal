## Why

The launcher reads two YAML files that describe a sidecar, and they disagree on
shape. The catalog groups each sidecar under its name, holding its container
beside the description and architectures that introduce it. The saved
`sidecars.yaml` holds a flat `containers` list instead, so a reader scans the
list to find one sidecar, and the launcher searches that list everywhere it
needs a sidecar by name.

`sidecars.yaml` is still unreleased, so its shape can match the catalog now
without a migration path.

## What Changes

- `sidecars.yaml` carries a top-level `sidecars` map keyed by sidecar name.
  Each entry holds a `container`, the shape a catalog entry already uses.
- The container name comes from the map key. A `name` inside `container` is
  accepted when it matches the key and reported as a validation error when it
  differs. The catalog already applies this rule.
- The launcher reads and writes saved sidecars in name order, so status
  listings and start remain deterministic.
- Validation errors name their field through the sidecar's key, for example
  `sidecars.mcp.container.image`.

No released version writes `sidecars.yaml`, so this is the first shape users
will see.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `sidecar-configuration`: the saved document's structure, and the container
  name rule that follows from keying entries by name.

## Impact

- The sidecar package gains the saved entry type and loses the search through a
  container list.
- Saved document reading and writing, and the field locations validation errors
  name.
- Sidecar operations resolve a name by map lookup, enable and disable each
  touch one entry, and ordered iteration replaces list order.
- Go tests covering the saved document, the Python test framework and launcher
  tests that write `sidecars.yaml`, the user and developer sidecar pages, and
  the unreleased changelog entry.
- Enablement, container creation, port publishing, reconciliation and cleanup
  keep their behavior.
