## Why

`exasol status` can wait indefinitely while acquiring the deployment lock or probing an unreachable database, preventing callers from reliably completing status checks.

## What Changes

- Bound `exasol status` to five seconds by default.
- Add a `--timeout` seconds option so callers can choose another positive bound.
- Report invalid timeout values without starting a status check.

## Capabilities

### New Capabilities

- `deployment-status-timeout`: Defines bounded status checks and caller-configurable timeout behavior.

### Modified Capabilities

None.

## Impact

This changes the `exasol status` CLI contract and its tests, help text, and changelog entry. It adds no dependency and does not change other commands.
