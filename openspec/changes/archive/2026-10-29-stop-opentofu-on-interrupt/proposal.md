## Why

When `exasol deploy` or `exasol destroy` is interrupted, the launcher exits at
once while OpenTofu is still stopping and recording what it changed. OpenTofu
can then be killed before it saves its state, or a following `exasol destroy`
can start before the state is saved, so a resource being created can survive.
When only the launcher receives the signal, OpenTofu keeps running in the
background. Interruption handling predates OpenSpec and is not specified.

## What Changes

- On SIGINT or SIGTERM during `exasol deploy` or `exasol destroy`, the launcher
  stops OpenTofu, waits for it to exit, and then exits with an error. This
  holds whether the signal reaches the whole terminal process group or only
  the launcher.
- The deployment is recorded as interrupted during the operation, and
  `exasol status` and `exasol info` report it.

## Capabilities

### Modified Capabilities

- `deployment-lifecycle-recovery`: interrupted deploy and destroy stop OpenTofu
  before the launcher exits and are recorded as interrupted.

## Impact

- The OpenTofu runner and the launcher's signal handling.
- A launcher test interrupting deploy and destroy with SIGINT and SIGTERM.
- The changelog.
