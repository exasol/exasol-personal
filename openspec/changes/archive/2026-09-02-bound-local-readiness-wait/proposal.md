## Why

Local install and start can wait five minutes after unsuccessful database connection attempts, although a healthy local database normally becomes ready within seconds. This delays failure recovery and can conceal reachability problems as a stuck deployment.

## What Changes

- Limit local database readiness waits for install and start to 30 seconds.
- Poll local database readiness every one to two seconds while it is starting.
- Retain the final database connection failure and add existing local reachability guidance when the failure is recognized as a network-wide local problem.
- Keep cloud deployment readiness timing unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `exasol-local-deployment`: Bound local install and start readiness behavior and failure reporting.

## Impact

The local deployment workflow, readiness polling, and their Go tests change. The CLI's cloud lifecycle behavior and external dependencies remain unchanged.
