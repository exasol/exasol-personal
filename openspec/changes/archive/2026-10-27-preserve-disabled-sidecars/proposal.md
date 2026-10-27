## Why

`exasol sidecar disable <name>` deletes the sidecar's entry from
`sidecars.yaml`. A user who edited the saved definition, for example to move the
MCP server to a free host port, loses that edit and has to repeat it after the
next enable, which starts again from the catalog template.

## What Changes

- Each saved entry in `sidecars.yaml` carries an `enabled` field beside its
  `container`. Enable writes `enabled: true`; an entry without the field counts
  as enabled.
- `exasol sidecar disable <name>` sets `enabled: false`, stops the sidecar, and
  removes its runtime resources while keeping the saved definition. Enabling it
  again uses that definition, including edited ports.
- `exasol sidecar disable <name> --remove` stops the sidecar and deletes its
  entry, so a later enable of a catalog name starts from the catalog template.
- Start, list, and status treat a disabled entry as disabled. Status without a
  name still reports every saved entry.
- Repeating either form of disable succeeds and reports that there is nothing
  to remove (`nothingToRemove: true` in JSON).
- The catalog rejects an `enabled` field, which belongs to a deployment's saved
  entry.

## Capabilities

### Modified Capabilities

- `sidecar-configuration`: saved entries record their enabled state, and
  disabling a name the deployment has not saved succeeds as a no-op.
- `sidecar-lifecycle`: disable keeps the saved definition, `--remove` deletes
  it, and start reconciles only enabled entries.

## Impact

- The sidecar package gains the `enabled` field on a saved definition and the
  catalog validation that rejects it.
- Enable, disable, list, status, and start reconciliation in the deploy package
  distinguish a saved entry from an enabled one.
- The `exasol sidecar disable` command gains `--remove`.
- The user sidecar page, the CLI reference, and the changelog.
