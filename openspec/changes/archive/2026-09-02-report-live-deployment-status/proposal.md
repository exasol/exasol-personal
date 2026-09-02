## Why

`exasol deployments list` reports persisted workflow state as status, so stopped or unreachable deployments can be presented as running. This makes the JSON output unsafe for clients that use it to select a deployment.

## What Changes

- Report the same live status that `exasol status` reports for each deployment.
- Check deployments concurrently under one five-second bound so listing latency does not grow with the number of deployments.
- Keep the database driver's temporary error suppression correct while status probes overlap.
- Preserve alphabetical output and tolerant handling of malformed deployment directories.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `deployment-directory-listing`: Require live, bounded, concurrent status resolution for listed deployments.

## Impact

This changes `exasol deployments list` status values, status-probe logging synchronization, tests, user documentation, and changelog. It relies on the bounded status behavior introduced for #311 and adds no dependency.
